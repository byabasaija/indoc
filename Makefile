# Rebuild the tree-sitter WebAssembly modules from pinned sources.
# Only needed when adding or updating a grammar; the .wasm files are committed.
grammars:
	docker build -q -t indoc-grammars internal/treesitter/build
	docker run --rm -v "$(CURDIR)":/work indoc-grammars sh internal/treesitter/build/build.sh

# Rebuild vendored browser libraries (Prism, Mermaid) from pinned versions.
vendor:
	sh tools/vendor/vendor.sh

.PHONY: grammars vendor
