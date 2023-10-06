.PHONY: install-local-bin install-configs
install-local-bin:
	./installers/install-local-bin.sh
	# TODO: Ensure that rust is installed
	make -C ./dotfiles-rs install

.PHONY: install-configs
install-configs:
	./installers/install-configs.sh

.PHONY: install-all
install-all:
	./installers/install-all.sh

.PHONY: pull-master
pull-master:
	git pull --ff origin master

.PHONY: update
update: pull-master install-all

.PHONY: clone-private
clone-private:
	git clone git@github.com:surajssd/dotfilesprivate.git
