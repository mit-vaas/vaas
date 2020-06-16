To run:

	mkdir items
	go get github.com/google/uuid
	go get github.com/mattn/go-sqlite3
	go get github.com/mitroadmaps/gomapinfer/common
	go run main.go

Go 1.11+ may be required. On most versions of Ubuntu you can use this PPA:

	sudo add-apt-repository ppa:longsleep/golang-backports
	sudo apt update
	sudo apt install golang

Once Vaas is running you can access it at http://127.0.0.1:8080

Dependencies
------------

Besides the Golang library dependencies, there are some external programs.

### ffmpeg

ffmpeg is used for reading, writing, and transcoding videos. The binary `ffmpeg`
must be in the PATH. On Ubuntu you can just:

	apt install ffmpeg

### youtube-dl

This is used for importing YouTube videos into Vaas. On Linux:

	sudo curl -L https://yt-dl.org/downloads/latest/youtube-dl -o /usr/local/bin/youtube-dl

### YOLOv3

There is a built-in model currently that uses YOLOv3. It relies on the version
with a one-line modification at https://github.com/uakfdotb/darknet (to print
bounding box coordinates) and expects a symlink to that repository in ./darknet.
This dependency is only needed if you use the built-in yolov3 node.

	git clone https://github.com/uakfdotb/darknet ~/darknet
	ln -s ~/darknet ./darknet
