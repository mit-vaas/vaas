import model

import json
import keras
import numpy
import struct
import sys

model_path = sys.argv[1]
num_classes = int(sys.argv[2])

m = model.get_model(num_classes)
m.load_weights(model_path)

stdin = sys.stdin.detach()
while True:
	buf = stdin.read(8)
	width, height = struct.unpack('>II', buf)
	buf = stdin.read(width*height*3)
	im = numpy.frombuffer(buf, dtype='uint8').reshape((height, width, 3))
	pred = m.predict(numpy.expand_dims(im, axis=0))[0]
	sys.stdout.buffer.write((json.dumps(pred.tolist()) + "\n").encode())
	sys.stdout.buffer.flush()
