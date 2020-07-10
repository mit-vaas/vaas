import model

import json
import keras
import numpy
import skimage.transform
import struct
import sys

model_path = sys.argv[1]
num_classes = int(sys.argv[2])
max_width = int(sys.argv[3])
max_height = int(sys.argv[4])
skip = int(sys.argv[5])
depth = int(sys.argv[6])

dims = model.get_dims(max_width, max_height)
dims = dims[skip:skip+depth]
m = model.get_model(num_classes, dims)
m.load_weights(model_path, by_name=True)

stdin = sys.stdin.detach()
while True:
	buf = stdin.read(8)
	width, height = struct.unpack('>II', buf)
	buf = stdin.read(width*height*3)
	im = numpy.frombuffer(buf, dtype='uint8').reshape((height, width, 3))
	if im.shape[0] != dims[0][1] or im.shape[1] != dims[0][0]:
		im = skimage.transform.resize(im, [dims[0][1], dims[0][0]], preserve_range=True).astype('uint8')
	inputs = [
		numpy.expand_dims(im, axis=0),
		numpy.ones((1, len(dims)), dtype='float32'),
	]
	pred = m.predict(inputs)
	if depth > 1:
		pred = pred[-1]
	pred = pred[0, :]
	sys.stdout.buffer.write((json.dumps(pred.tolist()) + "\n").encode())
	sys.stdout.buffer.flush()
