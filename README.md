# dot Files

This is set of scripts and configs which I use to do daily tasks. The `make` can be used to install those scripts and configs.

### Installation instructions

#### Install scripts

To install all the custom scripts to `~/.local/bin`, just run following command, from the root of this repository:

```bash
make install-local-bin
```

#### Install all

To install both scripts and configs just run:

```bash
make install-all
```

if you don't have `make` installed then just run:

```
./installers/install-all.sh
```

### First time quick setup

```bash
mkdir ~/git
cd ~/git
git clone https://github.com/surajssd/dotfiles
cd dotfiles
./installers/install-all.sh
```
