.DEFAULT_GOAL := help

# Symlink installers (delegate to scripts)
.PHONY: install-configs install-local-bin install-skills fetch-external-skills
# Go-tool installers (skip with an info message when dir absent)
.PHONY: install-azure-capacity-finder install-clawbox
# Orchestration / maintenance
.PHONY: install-all update pull-master test clone-private clone-azure-capacity-finder help

# Build+install a Go tool in $(1) if its dir exists; otherwise skip.
define go-install
@if [ -d "$(1)" ]; then \
	echo "⏳ Installing $(1) ..."; \
	cd "$(1)" && go install .; \
	echo "✅ $(1) installed to ~/go/bin"; \
else \
	echo "ℹ️  $(1) not present, skipping"; \
fi
endef

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "} {printf "  \033[36m%-32s\033[0m %s\n", $$1, $$2}'

install-configs: ## Install config files (shell, git, gpg, tmux, starship, k9s)
	./installers/install-configs.sh

install-local-bin: ## Install scripts to ~/.local/bin
	./installers/install-local-bin.sh

install-skills: ## Install agent skills to ~/.claude/skills and ~/.agents/skills
	./installers/install-skills.sh

fetch-external-skills: ## Download external skills (mattpocock, bastos) into skills/ — also run by 'make update'
	./installers/fetch-external-skills.sh

install-azure-capacity-finder: ## Install azure-capacity-finder Go tool (skipped if not cloned)
	$(call go-install,azure-capacity-finder)

install-clawbox: ## Install clawbox Go tool (skipped if absent)
	$(call go-install,clawbox)

install-all: install-configs install-local-bin install-skills install-azure-capacity-finder install-clawbox ## Install everything

update: pull-master fetch-external-skills install-all ## Pull latest, refresh external skills, then reinstall

pull-master: ## Pull latest from public + private + azure-capacity-finder
	git pull --ff origin master
	if [ -d dotfilesprivate ]; then cd dotfilesprivate && git pull --ff origin master; fi
	if [ -d azure-capacity-finder ]; then cd azure-capacity-finder && git pull --ff origin main; fi

test: ## Run clawbox tests
	$(MAKE) -C clawbox test

clone-private: ## Clone the private dotfiles repo
	git clone git@github.com:surajssd/dotfilesprivate.git

clone-azure-capacity-finder: ## Clone azure-capacity-finder
	git clone git@github.com:surajssd/azure-capacity-finder.git
