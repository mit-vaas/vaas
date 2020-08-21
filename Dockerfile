FROM docker.io/nvidia/cuda:10.0-cudnn7-devel-ubuntu18.04

RUN echo 'debconf debconf/frontend select Noninteractive' | debconf-set-selections

RUN apt-get update && \
	apt-get dist-upgrade -y && \
	apt-get install -y build-essential git python3-dev wget curl software-properties-common libmetis-dev ffmpeg && \
	add-apt-repository -y ppa:longsleep/golang-backports && \
	apt-get update && \
	apt-get install -y golang-1.13-go
RUN curl https://bootstrap.pypa.io/get-pip.py -o get-pip.py && \
	python3 get-pip.py && \
	pip3 install tensorflow-gpu==1.14 scikit-image scikit-video numpy keras==2.3.0

WORKDIR /usr/src/app

# yolov3
RUN git clone https://github.com/AlexeyAB/darknet.git
WORKDIR darknet
RUN git checkout b918bf0329c06ae67d9e00c8bcaf845f292b9d62 && \
	sed -i 's/GPU=.*/GPU=1/' Makefile && \
	sed -i 's/CUDNN=.*/CUDNN=1/' Makefile && \
	sed -i 's/LIBSO=.*/LIBSO=1/' Makefile && \
	make
RUN wget https://pjreddie.com/media/files/yolov3.weights
RUN wget https://pjreddie.com/media/files/darknet53.conv.74
WORKDIR /usr/src/app

# golang and youtube-dl dependencies
RUN ln -s /usr/lib/go-1.13/bin/go /usr/bin/go && \
	go get github.com/cpmech/gosl/graph && \
	go get github.com/google/uuid && \
	go get github.com/googollee/go-socket.io && \
	go get github.com/mattn/go-sqlite3 && \
	go get github.com/mitroadmaps/gomapinfer/common && \
	go get github.com/sasha-s/go-deadlock && \
	go get golang.org/x/image/font && \
	go get golang.org/x/image/font/basicfont && \
	go get golang.org/x/image/math/fixed && \
	curl -L https://yt-dl.org/downloads/latest/youtube-dl -o /usr/local/bin/youtube-dl

RUN mkdir vaas vaas/items
WORKDIR vaas
RUN ln -s /usr/src/app/darknet darknet

COPY ./ ./
RUN go build main.go && \
	go build machine.go && \
	go build container.go

EXPOSE 8080
CMD ./docker/entrypoint.sh
