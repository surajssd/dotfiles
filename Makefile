.PHONY: install-local-bin
install-local-bin:
	./installers/install-local-bin.sh

.PHONY: install-configs
install-configs:
	./installers/install-configs.sh

.PHONY: install-all
install-all:
	./installers/install-all.sh
