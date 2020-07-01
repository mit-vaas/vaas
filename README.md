To run:

	mkdir items
	go get github.com/google/uuid
	go get github.com/mattn/go-sqlite3
	go get github.com/mitroadmaps/gomapinfer/common
	go run main.go

And run worker in a separate terminal on the same machine:

	go build container.go
	go run machine.go

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

Data Model
----------

### Series

Data in Vaas are represented in a Series. A series is a time series, where some
data is associated with each timestep, except the time axis is segmented. For
video data, timesteps are video frames, and each segment of the time axis is a
different clip or video file in the same video collection.

`data_class.go`, `data_detection.go`, and similar files define different data
types that a series can store. For example, object detections are represented
by associating a list of bounding boxes with each timestep.

There are three types of Series:

- A Data Series represents raw data, e.g. video captured by a camera.
- A Labels Series consists of annotations hand-labeled by the user.
- An Output Series persists the outputs of a Node.

### Timelines, Segments, and Slices

Timelines and Segments represent the segmented time axis that a Series is
defined on. Multiple Series may be defined on the same time axis. For example,
a city may have several traffic cameras at a junction, and the video from each
camera may be contained in a separate Series, but all of the video (Series) can
share the same Timeline so that they can be analyzed together.

A Segment represents one contiguous time axis, and a Timeline consists of one
or more Segments.

Slices represent a range of a Segment, e.g. frames 100 to 200 in a video.

### Items

An item specifies where data in a Series is stored on disk. Video files are
stored as encoded video, and everything else is encoded as JSON. Items are
associated with a Slice, and store the data of the Series in that Slice.

For Data Series, the Slices on which items are defined generally correspond to
entire Segments, while items in Labels and Output Series generally are stored
in smaller chunks.

### Vectors

A vector is an ordered sequence of Series that are all defined on the same
Timeline. Vectors are used by queries and labels series to indicate their
inputs. Often, queries and labels series operate on a single video (i.e.,
operate on a 1-Vector), but some may involve multiple aligned series.

Query inputs are often vectors containing only Data Series, but this need not
always be the case.

### Queries and Nodes

A Query represents a data flow graph that composes a series of Nodes to
implement some analytics task.

Each Node specifies its parent(s), type, and data type. For example, to
classify traffic light colors, one may define a simple sequence of two nodes:

1. A node with type=Crop and data-type=Video that inputs Input[0] (the first
and only series in the query input vector) and outputs the video cropped around
the traffic light.
2. A node with type=Python and data-type=Class that inputs the cropped video and
processes it through a Python function to determine the light color.

Nodes may reference some Output Series where their outputs are persisted.

Queries specify a list of lists of nodes to output. Each sublist specifies how
to render one video: for example, the sublist [Input[0], YOLOv3] would render a
video with detections output by a YOLOv3 object detector node overlayed on
Input[0]. The sublists are stacked vertically in the visualization provided to
the user.

A query can also specify a selector node. If the output of the selector on some
slice of the input vector is empty, then the query should not be processed
further on that slice (and, in interactive setting, it should not be shown to
the user).

### Labels Series

A Labels Series is a special Series that supports annotation. To this end, the
Labels Series specifies a vector of inputs that the annotation is performed on.
For example, to annotate for an object detector, the input should be a single
video (from which individual frames will be sampled for labeling) and the data
type of the series should be Detection.

The inputs can contain non-Data Series. For example, after producing some car
tracks, one may want to train a model to classify tracks for a certain feature
of interest. Then one can create a Labels Series where the input is the Output
Series of the tracker node, and the data type is track.

Annotation itself requires a specialized front-end module, which the user
selects when creating the Labels Series. Oftentimes these modules support only
one particular vector of input types and series type.
