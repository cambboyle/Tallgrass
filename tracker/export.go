package tracker

import (
	"sort"
	"time"
)

// RunExport is one run's whole history, for backups (cloud sync).
type RunExport struct {
	Game      string    `json:"game"`
	TrainerID uint32    `json:"trainerId"`
	Player    string    `json:"player"`
	StartedAt time.Time `json:"startedAt"`
	Mons      []Mon     `json:"mons"`
	Areas     []Area    `json:"areas"`
	Caught    []int     `json:"caught"` // national numbers
	Events    []Event   `json:"events"` // oldest first
}

// Export reads every run in the store.
func (s *Store) Export() ([]RunExport, error) {
	rows, err := s.db.Query(`SELECT id, game, trainer_id, player_name, started_at FROM runs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	type run struct {
		id int64
		RunExport
	}
	var runs []run
	for rows.Next() {
		var r run
		var started string
		if err := rows.Scan(&r.id, &r.Game, &r.TrainerID, &r.Player, &started); err != nil {
			rows.Close()
			return nil, err
		}
		r.StartedAt, _ = time.Parse(timeFormat, started)
		runs = append(runs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []RunExport{}
	for _, r := range runs {
		mons, err := s.loadMons(r.id)
		if err != nil {
			return nil, err
		}
		for _, m := range mons {
			r.Mons = append(r.Mons, *m)
		}
		sort.Slice(r.Mons, func(i, j int) bool { return r.Mons[i].ObtainedAt.Before(r.Mons[j].ObtainedAt) })
		areas, err := s.loadAreas(r.id)
		if err != nil {
			return nil, err
		}
		for _, a := range areas {
			r.Areas = append(r.Areas, *a)
		}
		sort.Slice(r.Areas, func(i, j int) bool { return r.Areas[i].FirstVisit.Before(r.Areas[j].FirstVisit) })
		caught, err := s.loadDexCaught(r.id)
		if err != nil {
			return nil, err
		}
		for n := range caught {
			r.Caught = append(r.Caught, n)
		}
		sort.Ints(r.Caught)
		if r.Events, err = s.loadEvents(r.id); err != nil {
			return nil, err
		}
		sort.SliceStable(r.Events, func(i, j int) bool { return r.Events[i].ID < r.Events[j].ID })
		out = append(out, r.RunExport)
	}
	return out, nil
}
