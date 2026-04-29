.PHONY: build frontend server placeholder clean

# Full build: compile the frontend and produce a single Go binary that
# embeds it. Output: ./server
build: frontend server

frontend:
	cd frontend && npm ci && npm run build
	mkdir -p cmd/server/frontend_dist
	find cmd/server/frontend_dist -mindepth 1 -not -name .gitignore -exec rm -rf {} +
	cp -R frontend/build/. cmd/server/frontend_dist/

# Ensure the embed directory has at least one file so `go build` succeeds
# without a frontend build (CI Go-only checks, fresh clones, etc.).
placeholder:
	@mkdir -p cmd/server/frontend_dist
	@test -f cmd/server/frontend_dist/index.html || \
		printf '<!doctype html><html><body>Frontend not built. Run `make build`.</body></html>\n' \
		> cmd/server/frontend_dist/index.html

server: placeholder
	CGO_ENABLED=0 go build -o server ./cmd/server

clean:
	rm -f server
	mkdir -p cmd/server/frontend_dist
	find cmd/server/frontend_dist -mindepth 1 -not -name .gitignore -exec rm -rf {} +
