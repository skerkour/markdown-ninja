NAME = $(shell cat package.json | grep name | cut -d '"' -f4)
VERSION = $(shell cat package.json | grep version | cut -d '"' -f4)
DATE = $(shell date +"%Y-%m-%d")
DIST_DIR = dist

.PHONY: build
build:
	make exif
	npm run build
	# for now we don't need the web manifest
	rm -rf $(DIST_DIR)/.vite

.PHONY: exif
exif:
	exiftool -overwrite_original -recurse -all= public/* || exit 0

.PHONY: install
install:
	npm install --no-scripts --ignore-scripts

.PHONY: install_ci
install_ci:
	npm config set ignore-scripts true
	npm ci --ignore-scripts

.PHONY: dev
dev:
	npm run dev

.PHONY: clean
clean:
	rm -rf $(DIST_DIR) node_modules


.PHONY: check
check:
	npm run type-check


.PHONY: re
re: clean build

.PHONY: update_deps
update_deps:
	npm update
	make build
	npm outdated

.PHONY: deploy
deploy:
	npm run deploy
