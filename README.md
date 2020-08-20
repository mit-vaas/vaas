Vaas
====

Vaas is a video analytics system for large-scale video datasets. It provides a web interface that incorporates a query composition tool and data exploration tool to accelerate the development of complex query pipelines. Vaas also provides a human-in-the-loop query optimization framework to accelerate query execution.

Website: https://vaas.csail.mit.edu

Quickstart
----------

The fastest way to get started is with Docker. First, install [nvidia-docker](https://github.com/NVIDIA/nvidia-docker); on Ubuntu (tested on 16.04):

	distribution=$(. /etc/os-release;echo $ID$VERSION_ID)
	curl -s -L https://nvidia.github.io/nvidia-docker/gpgkey | sudo apt-key add -
	curl -s -L https://nvidia.github.io/nvidia-docker/$distribution/nvidia-docker.list | sudo tee /etc/apt/sources.list.d/nvidia-docker.list
	sudo apt update && sudo apt install -y docker.io nvidia-container-toolkit
	sudo systemctl restart docker

Then:

	git clone https://github.com/mit-vaas/vaas.git
	cd vaas
	docker build -t mit-vaas/vaas .
	docker run -p 8080:8080 mit-vaas/vaas

Access your Vaas deployment at http://localhost:8080.

If you want to run it without Docker, first install CUDA 10.0 and cuDNN 7.6, then follow the RUN commands in docker/Dockerfile.

Resources
---------

- VLDB 2020 demo [paper](https://favyen.com/vaas.pdf) and [video](https://www.youtube.com/watch?v=cDsZKJUpLF4)
- Website: https://vaas.csail.mit.edu
