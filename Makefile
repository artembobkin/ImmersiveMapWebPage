# ImmersiveMap website — build helpers.
#
# The output of `make site` is the whole deployable artefact: a directory of
# static files with nothing behind them.

OUT      ?= dist
BASE_URL ?= https://immersivemap.dev
PORT     ?= 8000

.PHONY: site check serve code-css clean

site: ## render the landing page, articles and tutorials into $(OUT)
	go run . -content articles -tutorials tutorials -site site -out $(OUT) -base-url $(BASE_URL)

# Drafts are skipped by a normal build, so this is how you look at one.
drafts: ## same, including articles marked draft: true
	go run . -content articles -tutorials tutorials -site site -out $(OUT) -base-url $(BASE_URL) -drafts

check:
	go vet ./...
	go test ./...

serve: site ## build, then serve the output locally
	@echo "http://localhost:$(PORT)"
	@cd $(OUT) && python3 -m http.server $(PORT)

# The syntax-highlighting stylesheet is generated from the Chroma theme rather
# than written by hand; regenerate it after changing the theme.
code-css:
	go run . -emit-code-css > site/templates/code.css

clean:
	rm -rf $(OUT)
