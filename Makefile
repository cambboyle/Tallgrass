PI      ?= pi@192.168.0.227
PI_DIR  := /home/pi/tallgrass
BIN     := tallgrass
PI_BIN  := build/$(BIN)-linux-armv7
# Builds made here are development builds: they never offer to update
# themselves. Releases come from GitHub Actions (tag vX.Y.Z).
VERSION := dev-$(shell git rev-parse --short HEAD 2>/dev/null || echo local)

.PHONY: run test build-pi deploy install-service logs status

run:
	go run . -host $(lastword $(subst @, ,$(PI))):55355

test:
	go test ./...

build-pi:
	GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(PI_BIN) .

# Copy to a temp name and rename, because Linux won't overwrite a running binary.
deploy: test build-pi
	ssh $(PI) 'mkdir -p $(PI_DIR)'
	scp $(PI_BIN) $(PI):$(PI_DIR)/$(BIN).new
	ssh $(PI) 'mv $(PI_DIR)/$(BIN).new $(PI_DIR)/$(BIN) && (systemctl is-enabled --quiet $(BIN) 2>/dev/null && sudo systemctl restart $(BIN) || true)'
	@echo "Deployed: http://$(lastword $(subst @, ,$(PI))):8080"

# One-time: install and start the systemd service (run after the first deploy).
install-service:
	scp deploy/$(BIN).service $(PI):/tmp/$(BIN).service
	ssh $(PI) 'sudo mv /tmp/$(BIN).service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable --now $(BIN)'

logs:
	ssh $(PI) 'journalctl -u $(BIN) -f -n 50'

status:
	ssh $(PI) 'systemctl status $(BIN) --no-pager'
