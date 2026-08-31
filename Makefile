.DEFAULT_GOAL := help

# Symlink installers (delegate to scripts)
.PHONY: install-configs install-local-bin install-skills install-rules install-private fetch-external-skills fetch-external-rules
# Orchestration / maintenance
.PHONY: install-all update pull-master clone-private help

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "} {printf "  \033[36m%-32s\033[0m %s\n", $$1, $$2}'

install-configs: ## Install config files (shell, git, gpg, tmux, starship, k9s)
	./installers/install-configs.sh

install-local-bin: ## Install scripts to ~/.local/bin
	./installers/install-local-bin.sh

install-skills: ## Install agent skills to ~/.claude/skills and ~/.agents/skills
	./installers/install-skills.sh

install-rules: ## Install agent rules to ~/.claude/rules
	./installers/install-rules.sh

install-private: ## Install the optional private dotfiles
	@if [ -x dotfilesprivate/install.sh ]; then ./dotfilesprivate/install.sh; fi

fetch-external-skills: ## Download external skills (mattpocock, bastos, blader) into skills/ — also run by 'make update'
	./installers/fetch-external-skills.sh

fetch-external-rules: ## Download external rules (abatilo) into rules/ — also run by 'make update'
	./installers/fetch-external-rules.sh

install-all: install-configs install-local-bin install-skills install-rules ## Install everything
	$(MAKE) install-private

update: pull-master fetch-external-skills fetch-external-rules install-all ## Pull latest, refresh external skills + rules, then reinstall

pull-master: ## Pull latest from public + private
	git pull --ff origin master
	if [ -d dotfilesprivate ]; then cd dotfilesprivate && git pull --ff origin master; fi

clone-private: ## Clone the private dotfiles repo
	git clone git@github.com:surajssd/dotfilesprivate.git
