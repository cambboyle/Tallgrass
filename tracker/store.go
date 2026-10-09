// Package tracker turns successive game snapshots into Nuzlocke events
// (catches, faints, new areas...) and keeps them in SQLite.
package tracker

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure Go, so cross-compiling for the Pi needs no cgo
)

// Store is the SQLite database. Writes happen only when something changes,
// to spare the Pi's SD card.
type Store struct {
	db *sql.DB
}

var migrations = []string{
	`CREATE TABLE runs (
		id          INTEGER PRIMARY KEY,
		game        TEXT    NOT NULL,
		trainer_id  INTEGER NOT NULL,
		player_name TEXT    NOT NULL,
		started_at  TEXT    NOT NULL,
		UNIQUE (game, trainer_id, player_name)
	);
	CREATE TABLE mons (
		run_id       INTEGER NOT NULL REFERENCES runs(id),
		personality  INTEGER NOT NULL,
		species      INTEGER NOT NULL,
		species_name TEXT    NOT NULL,
		nickname     TEXT    NOT NULL,
		level        INTEGER NOT NULL,
		is_egg       INTEGER NOT NULL,
		met_section  INTEGER NOT NULL,
		met_area     TEXT    NOT NULL,
		obtained_at  TEXT    NOT NULL,
		fainted_at   TEXT,
		PRIMARY KEY (run_id, personality)
	);
	CREATE TABLE areas (
		run_id      INTEGER NOT NULL REFERENCES runs(id),
		section     INTEGER NOT NULL,
		name        TEXT    NOT NULL,
		first_visit TEXT    NOT NULL,
		PRIMARY KEY (run_id, section)
	);
	CREATE TABLE dex_caught (
		run_id   INTEGER NOT NULL REFERENCES runs(id),
		national INTEGER NOT NULL,
		PRIMARY KEY (run_id, national)
	);
	CREATE TABLE events (
		id          INTEGER PRIMARY KEY,
		run_id      INTEGER NOT NULL REFERENCES runs(id),
		at          TEXT    NOT NULL,
		kind        TEXT    NOT NULL,
		personality INTEGER,
		species     INTEGER,
		section     INTEGER,
		area        TEXT    NOT NULL,
		subject     TEXT    NOT NULL, -- the Pokémon, e.g. "Blaze (Cyndaquil)"
		text        TEXT    NOT NULL
	);
	CREATE INDEX events_by_run ON events (run_id, id);`,

	// 2: per-game settings, e.g. Nuzlocke mode.
	`CREATE TABLE settings (
		game  TEXT NOT NULL,
		key   TEXT NOT NULL,
		value TEXT NOT NULL,
		PRIMARY KEY (game, key)
	);`,
}

// Open opens (creating if needed) the database at path and migrates it.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // only the poller writes; avoids SQLITE_BUSY

	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		db.Close()
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	for i := version; i < len(migrations); i++ {
		if _, err := db.Exec(migrations[i]); err != nil {
			db.Close()
			return nil, fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

const timeFormat = time.RFC3339

// run finds or creates the run for a save file. created reports a new run.
func (s *Store) run(game string, trainerID uint32, player string, now time.Time) (id int64, created bool, err error) {
	err = s.db.QueryRow(`SELECT id FROM runs WHERE game = ? AND trainer_id = ? AND player_name = ?`,
		game, trainerID, player).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}
	res, err := s.db.Exec(`INSERT INTO runs (game, trainer_id, player_name, started_at) VALUES (?, ?, ?, ?)`,
		game, trainerID, player, now.Format(timeFormat))
	if err != nil {
		return 0, false, err
	}
	id, err = res.LastInsertId()
	return id, true, err
}

func (s *Store) loadMons(runID int64) (map[uint32]*Mon, error) {
	rows, err := s.db.Query(`SELECT personality, species, species_name, nickname, level, is_egg,
		met_section, met_area, obtained_at, fainted_at FROM mons WHERE run_id = ?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mons := map[uint32]*Mon{}
	for rows.Next() {
		var m Mon
		var obtained string
		var fainted sql.NullString
		if err := rows.Scan(&m.Personality, &m.Species, &m.SpeciesName, &m.Nickname, &m.Level, &m.IsEgg,
			&m.MetSection, &m.MetArea, &obtained, &fainted); err != nil {
			return nil, err
		}
		m.ObtainedAt, _ = time.Parse(timeFormat, obtained)
		if fainted.Valid {
			t, _ := time.Parse(timeFormat, fainted.String)
			m.FaintedAt = &t
		}
		mons[m.Personality] = &m
	}
	return mons, rows.Err()
}

func (s *Store) saveMon(runID int64, m *Mon) error {
	var fainted any
	if m.FaintedAt != nil {
		fainted = m.FaintedAt.Format(timeFormat)
	}
	_, err := s.db.Exec(`INSERT INTO mons (run_id, personality, species, species_name, nickname, level, is_egg,
			met_section, met_area, obtained_at, fainted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (run_id, personality) DO UPDATE SET species = excluded.species,
			species_name = excluded.species_name, nickname = excluded.nickname, level = excluded.level,
			is_egg = excluded.is_egg, fainted_at = excluded.fainted_at`,
		runID, m.Personality, m.Species, m.SpeciesName, m.Nickname, m.Level, m.IsEgg,
		m.MetSection, m.MetArea, m.ObtainedAt.Format(timeFormat), fainted)
	return err
}

func (s *Store) loadAreas(runID int64) (map[int]*Area, error) {
	rows, err := s.db.Query(`SELECT section, name, first_visit FROM areas WHERE run_id = ?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	areas := map[int]*Area{}
	for rows.Next() {
		var a Area
		var first string
		if err := rows.Scan(&a.Section, &a.Name, &first); err != nil {
			return nil, err
		}
		a.FirstVisit, _ = time.Parse(timeFormat, first)
		areas[a.Section] = &a
	}
	return areas, rows.Err()
}

func (s *Store) saveArea(runID int64, a *Area) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO areas (run_id, section, name, first_visit) VALUES (?, ?, ?, ?)`,
		runID, a.Section, a.Name, a.FirstVisit.Format(timeFormat))
	return err
}

func (s *Store) loadDexCaught(runID int64) (map[int]bool, error) {
	rows, err := s.db.Query(`SELECT national FROM dex_caught WHERE run_id = ?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	caught := map[int]bool{}
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		caught[n] = true
	}
	return caught, rows.Err()
}

func (s *Store) saveDexCaught(runID int64, nationals []int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, n := range nationals {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO dex_caught (run_id, national) VALUES (?, ?)`, runID, n); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) addEvent(runID int64, e *Event) error {
	res, err := s.db.Exec(`INSERT INTO events (run_id, at, kind, personality, species, section, area, subject, text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, e.At.Format(timeFormat), e.Kind, nullIfZero(int64(e.Personality)), nullIfZero(int64(e.Species)),
		e.Section, e.Area, e.Subject, e.Text)
	if err != nil {
		return err
	}
	e.ID, err = res.LastInsertId()
	return err
}

// loadEvents returns the run's events, oldest first.
func (s *Store) loadEvents(runID int64) ([]Event, error) {
	rows, err := s.db.Query(`SELECT id, at, kind, COALESCE(personality, 0), COALESCE(species, 0), section, area, subject, text
		FROM events WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var at string
		if err := rows.Scan(&e.ID, &at, &e.Kind, &e.Personality, &e.Species, &e.Section, &e.Area, &e.Subject, &e.Text); err != nil {
			return nil, err
		}
		e.At, _ = time.Parse(timeFormat, at)
		events = append(events, e)
	}
	return events, rows.Err()
}

func nullIfZero(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// Setting reads a per-game setting; "" if unset.
func (s *Store) Setting(game, key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE game = ? AND key = ?`, game, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(game, key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (game, key, value) VALUES (?, ?, ?)
		ON CONFLICT (game, key) DO UPDATE SET value = excluded.value`, game, key, value)
	return err
}
