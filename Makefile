.PHONY: build build-ui build-go clean

build: build-ui build-go

build-ui:
	cd ui && npm run build
	rm -rf cmd/flow/ui_dist
	cp -r ui/dist cmd/flow/ui_dist

build-go: build-ui
	go build -o flow ./cmd/flow/

clean:
	rm -rf flow cmd/flow/ui_dist ui/dist
