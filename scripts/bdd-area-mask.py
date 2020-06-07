import json
import numpy
import os
import skimage.draw, skimage.io, skimage.transform
import sqlite3
import subprocess

# import driveable area labels into vaas
# we create two vaas videos, both with single-image clips:
# (1) original RGB images (scaled down)
# (2) output masks
# for now the latter images are 3-channel even though they can be 1-channel,
#    since vaas only supports 3-channel

SRC_VIDEO = 5
LABEL_VIDEO = 6
SET_ID = 7
WIDTH = 640
HEIGHT = 360

print('reading json')
with open('/data2/bdd/bdd100k/labels/bdd100k_labels_images_train.json', 'r') as f:
	images = json.load(f)

conn = sqlite3.connect('skyhook.sqlite3')
cursor = conn.cursor()

print('processing images')
for image in images:
	fname = image['name']
	print(fname)
	polygons = []
	for label in image['labels']:
		if label['category'] != 'drivable area':
			continue
		areaType = label['attributes']['areaType']
		if areaType != 'direct' and areaType != 'alternative':
			continue
		assert len(label['poly2d']) == 1
		r = [p[1]/2 for p in label['poly2d'][0]['vertices']]
		c = [p[0]/2 for p in label['poly2d'][0]['vertices']]
		polygons.append((r, c, areaType))
	if len(polygons) == 0:
		continue

	im = skimage.io.imread('/data2/bdd/bdd100k/images/100k/train/{}'.format(fname))
	im = skimage.transform.resize(im, [HEIGHT, WIDTH], preserve_range=True).astype('uint8')
	label_im = numpy.zeros((HEIGHT, WIDTH, 3), dtype='uint8')
	for r, c, areaType in polygons:
		rr, cc = skimage.draw.polygon(r, c, shape=(HEIGHT, WIDTH))
		if areaType == 'direct':
			label_im[rr, cc, :] = [255, 0, 0]
		else:
			label_im[rr, cc, :] = [0, 255, 0]

	cursor.execute('INSERT INTO clips (video_id, nframes, width, height) VALUES (?, 1, ?, ?)', (SRC_VIDEO, WIDTH, HEIGHT))
	src_id = cursor.lastrowid
	cursor.execute('INSERT INTO clips (video_id, nframes, width, height) VALUES (?, 1, ?, ?)', (LABEL_VIDEO, WIDTH, HEIGHT))
	label_id = cursor.lastrowid
	cursor.execute('INSERT INTO labels (set_id, clip_id, start, end, out_clip_id) VALUES (?, ?, 0, 1, ?)', (SET_ID, src_id, label_id))

	os.makedirs('clips/{}/{}/'.format(SRC_VIDEO, src_id))
	skimage.io.imsave('clips/{}/{}/000001.jpg'.format(SRC_VIDEO, src_id), im)
	os.makedirs('clips/{}/{}/'.format(LABEL_VIDEO, label_id))
	skimage.io.imsave('clips/{}/{}/000001.jpg'.format(LABEL_VIDEO, label_id), label_im)

conn.commit()
conn.close()
