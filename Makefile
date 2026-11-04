# ImmersiveMap website — build helpers.
#
# The output of `make site` is the whole deployable artefact: a directory of
# static files with nothing behind them.

OUT      ?= dist
BASE_URL ?= https://immersivemap.dev
PORT     ?= 8000

# The landing page is the engine's README, read from landing/README.md (kept
# in step with GitHub by the follow-readme workflow). Point README at another
# file to preview an edit before it is pushed over there:
# `make site README=../ImmersiveMap/README.md`.
README   ?=
READMEFLAG = $(if $(README),-readme $(README))

.PHONY: site drafts check serve code-css clean

site: ## render the README, the articles and the tutorials into $(OUT)
	go run . -content articles -tutorials tutorials -site site -out $(OUT) -base-url $(BASE_URL) $(READMEFLAG)

# Drafts are skipped by a normal build, so this is how you look at one.
drafts: ## same, including articles marked draft: true
	go run . -content articles -tutorials tutorials -site site -out $(OUT) -base-url $(BASE_URL) $(READMEFLAG) -drafts

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
