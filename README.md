## KOQ

### Project description of KOQ

KOQ is a little helper tool to rescue mit slowly dying DVDs as MP4 streams for future enjoyment. It can be run with
LINUX and WINDOWS. KOQ relies heavliy on the two well known LINUX tools LSDVD and HANDBRAKE.

### LINUX install

To run with LINUX please follow the following steps. Description is for Ubuntu 20.04:

* Install any suitable LINUX distro of your choise.
* Install latest LINUX OS patches: sudo apt update && sudo apt upgrade
* Install the tool LSDVD: sudo apt install lsdvd
* Install the tool HANDBRAKE: sudo apt install handbrake
* Install the Google GO tool chain from https://golang.org/dl/
* Download the KOQ source code to a temporary dirctory: git clone https://github.com/mpetavy/koq
* CD into the temporary dirctory
* Compile the code: go install
* Show flag documentation: koq -?

### Sample execution

```
koq -i /dev/dvd -o ~/Videos/KingOfQueens
```

### License

All software is copyright and protected by the Apache License, Version 2.0.
https://www.apache.org/licenses/LICENSE-2.0