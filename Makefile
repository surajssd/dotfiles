.PHONY: install-local-bin
install-local-bin:
	./installers/install-local-bin.sh

.PHONY: install-configs
install-configs:
	./installers/install-configs.sh

.PHONY: install-skills
install-skills:
	./installers/install-skills.sh

.PHONY: install-azure-capacity-finder
install-azure-capacity-finder:
	cd azure-capacity-finder && go install .

.PHONY: install-clawbox
install-clawbox:
	cd clawbox && go install .

.PHONY: test
test:
	cd clawbox && $(MAKE) test

.PHONY: install-all
install-all:
	./installers/install-all.sh

.PHONY: pull-master
pull-master:
	git pull --ff origin master
	if [ -d dotfilesprivate ]; then cd dotfilesprivate && git pull --ff origin master; fi
	if [ -d azure-capacity-finder ]; then cd azure-capacity-finder && git pull --ff origin main; fi

.PHONY: update
update: pull-master install-all

.PHONY: clone-private
clone-private:
	git clone git@github.com:surajssd/dotfilesprivate.git

.PHONY: clone-azure-capacity-finder
clone-azure-capacity-finder:
	git clone git@github.com:surajssd/azure-capacity-finder.git
