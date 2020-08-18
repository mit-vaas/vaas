import json
import numpy
import math
import os
import struct
import sys

os.chdir('./darknet/')
sys.path.append('.')
import darknet

config_path = sys.argv[1]
weight_path = sys.argv[2]
meta_path = sys.argv[3]

my_net = darknet.load_net_custom(config_path.encode('ascii'), weight_path.encode('ascii'), 0, 1)
my_meta = darknet.load_meta(meta_path.encode('ascii'))

stdin = sys.stdin.detach()
while True:
	buf = stdin.read(8)
	width, height = struct.unpack('>II', buf)
	buf = stdin.read(width*height*3)
	im = numpy.frombuffer(buf, dtype='uint8').reshape((height, width, 3))

	darknet_image = darknet.make_image(im.shape[1], im.shape[0], 3)
	darknet.copy_image_from_bytes(darknet_image, im.tobytes())
	outputs = darknet.detect_image(my_net, my_meta, darknet_image, thresh=0.1)
	darknet.free_image(darknet_image)

	detections = []
	for cls, score, (cx, cy, w, h) in outputs:
		detections.append({
			'Class': str(cls),
			'Score': float(score),
			'Left': int(cx-w/2),
			'Right': int(cx+w/2),
			'Top': int(cy-h/2),
			'Bottom': int(cy+h/2),
		})
	data = {
		'Detections': detections,
		'CanvasDims': [im.shape[1], im.shape[0]],
	}

	sys.stdout.buffer.write(b'\nskyhook-yolov3' + json.dumps(data).encode() + b'\n')
	sys.stdout.buffer.flush()
