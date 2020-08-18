import model

import json
import keras
import numpy
import skimage.transform
import struct
import sys

sys.path.append('.')
import skyhook_pylib as lib

model_path = sys.argv[1]
num_classes = int(sys.argv[2])
width = int(sys.argv[3])
height = int(sys.argv[4])

m = model.get_model(num_classes, width, height)
m.load_weights(model_path)

@lib.per_frame_decorate
def f(im):
	if im.shape[0] != height or im.shape[1] != width:
		im = skimage.transform.resize(im, [height, width], preserve_range=True).astype('uint8')
	pred = m.predict(numpy.expand_dims(im, axis=0))[0]
	cls = int(pred.argmax())
	return cls

lib.run(f)
