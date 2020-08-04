import json
import os
import random
import sys
import subprocess

sys.path.append('/home/ubuntu/map-tensorflow/new/discoverlib')
import geom

export_path = sys.argv[1]
yolo_path = sys.argv[2]

LABEL_WIDTH = 1280
LABEL_HEIGHT = 720

examples = []
for fname in os.listdir(export_path):
	if not fname.endswith('_0.jpg'):
		continue
	label = fname.split('_0.jpg')[0]
	example_path = yolo_path + 'images/' + label

	rect = geom.Rectangle(
		geom.Point(0, 0),
		geom.Point(LABEL_WIDTH, LABEL_HEIGHT),
	)
	with open(export_path+label+'_1.json', 'r') as f:
		detections = json.load(f)[0]
	objects = [geom.Point(d['left'], d['top']).bounds().add_tol(30) for d in detections]
	objects = [rect.clip_rect(obj_rect) for obj_rect in objects]
	crop_objects = []
	for obj_rect in objects:
		start = geom.FPoint(float(obj_rect.start.x) / LABEL_WIDTH, float(obj_rect.start.y) / LABEL_HEIGHT)
		end = geom.FPoint(float(obj_rect.end.x) / LABEL_WIDTH, float(obj_rect.end.y) / LABEL_HEIGHT)
		crop_objects.append((start.add(end).scale(0.5), end.sub(start)))
	crop_lines = ['0 {} {} {} {}'.format(center.x, center.y, size.x, size.y) for center, size in crop_objects]

	subprocess.call(['cp', export_path+label+'_0.jpg', example_path+'.jpg'])
	with open(example_path+'.txt', 'w') as f:
		f.write("\n".join(crop_lines) + "\n")
	examples.append(example_path+'.jpg')

random.shuffle(examples)
num_val = len(examples)//10
val_set = examples[0:num_val]
train_set = examples[num_val:]
with open(yolo_path+'train.txt', 'w') as f:
	f.write("\n".join(train_set) + "\n")
with open(yolo_path+'test.txt', 'w') as f:
	f.write("\n".join(val_set) + "\n")
with open(yolo_path+'valid.txt', 'w') as f:
	f.write("\n".join(val_set) + "\n")
