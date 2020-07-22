import model

import json
import math
import numpy
import os
import pickle
import random
import skimage.io, skimage.transform
import skvideo.io
import sys
import tensorflow as tf
import time

export_path = sys.argv[1]
model_path = sys.argv[2]
skip = int(sys.argv[3])

NORM = 1000.0
CROP_SIZE = 64
MATCH_LENGTHS = [4, 8, 16]

labels = []
for fname in os.listdir(export_path):
	if not fname.endswith('_0.mp4'):
		continue
	label = fname.split('_0.mp4')[0]
	labels.append(label)

print('loading infos and matches')
all_frame_data = {}
for label in labels:
	print('... {}'.format(label))
	video_fname = export_path + label + '_0.mp4'
	json_fname = export_path + label + '_1.json'
	match_fname = export_path + label + '_match.json'

	with open(json_fname, 'r') as f:
		detections = json.load(f)
	with open(match_fname, 'r') as f:
		raw_matches = json.load(f)
		matches = {}
		for match_len in raw_matches:
			matches[int(match_len)] = {}
			for frame_idx in raw_matches[match_len]:
				matches[int(match_len)][int(frame_idx)] = {}
				for left_idx in raw_matches[match_len][frame_idx]:
					matches[int(match_len)][int(frame_idx)][int(left_idx)] = raw_matches[match_len][frame_idx][left_idx]

	# get detection crops (infos)
	infos = {}
	vreader = skvideo.io.vreader(video_fname)
	for frame_idx, im in enumerate(vreader):
		dlist = detections[frame_idx]
		if not dlist:
			continue

		info = []
		for idx, d in enumerate(dlist):
			if d['right']-d['left'] < 4 or d['bottom']-d['top'] < 4:
				continue
			crop = im[d['top']:d['bottom'], d['left']:d['right'], :]
			resize_factor = min([float(CROP_SIZE) / crop.shape[0], float(CROP_SIZE) / crop.shape[1]])
			resize_shape = [int(crop.shape[0] * resize_factor), int(crop.shape[1] * resize_factor)]
			if resize_shape[0] == 0 or resize_shape[1] == 0:
				continue
			crop = skimage.transform.resize(crop, resize_shape, preserve_range=True).astype('uint8')
			fix_crop = numpy.zeros((CROP_SIZE, CROP_SIZE, 3), dtype='uint8')
			fix_crop[0:crop.shape[0], 0:crop.shape[1], :] = crop
			d['width'] = float(d['right']-d['left'])/NORM
			d['height'] = float(d['bottom']-d['top'])/NORM
			info.append((d, fix_crop, idx))

		infos[frame_idx] = info

	all_frame_data[label] = (infos, matches)

def get_loc(detection):
	cx = (detection['left'] + detection['right']) / 2
	cy = (detection['top'] + detection['bottom']) / 2
	cx = float(cx) / NORM
	cy = float(cy) / NORM
	return cx, cy

def get_stuff(infos, matches):
	def per_info(info):
		images = []
		boxes = numpy.zeros((len(info), 4), dtype='float32')
		for i, (detection, crop, _) in enumerate(info):
			images.append(crop)
			cx, cy = get_loc(detection)
			boxes[i, :] = [cx, cy, detection['width'], detection['height']]
		detections = [get_loc(detection) for detection, _, _ in info]
		return images, boxes, detections, len(info)

	all_images = []
	all_boxes = []
	all_detections = []
	all_counts = []
	for i, info in enumerate(infos):
		images, boxes, detections, count = per_info(info)
		all_images.append(images)
		all_boxes.append(boxes)
		all_detections.append(detections)
		all_counts.append(count)

	all_masks = []
	for i, match_len in enumerate(MATCH_LENGTHS):
		last_idx = match_len
		mask = numpy.zeros((len(infos[0]), len(infos[last_idx])+1), dtype='float32')
		mask[:, len(infos[last_idx])] = 1
		first_map = {}
		for j, (_, _, orig_idx) in enumerate(infos[0]):
			first_map[orig_idx] = j
		last_map = {}
		for j, (_, _, orig_idx) in enumerate(infos[last_idx]):
			last_map[orig_idx] = j
		for left_idx in matches[i]:
			if left_idx not in first_map:
				continue
			for right_idx in matches[i][left_idx]:
				if right_idx not in last_map:
					continue
				mask[first_map[left_idx], last_map[right_idx]] = 1
		all_masks.append(mask.flatten())

	return all_images, all_boxes, all_detections, all_counts, all_masks

# random info generator for selecting negative examples
print('preparing random info generator')
labels_and_weights = [(label, len(all_frame_data[label][0])) for label in all_frame_data.keys()]
def get_random_info(exclude_label):
	labels = [label for label in all_frame_data.keys() if label != exclude_label]
	weights = [len(all_frame_data[label][0]) for label in labels]
	weight_sum = sum(weights)
	weights = [float(x)/float(weight_sum) for x in weights]
	while True:
		label = numpy.random.choice(labels, p=weights)
		frame_infos = all_frame_data[label][0]
		frame_idx = random.choice(list(frame_infos.keys()))
		if len(frame_infos[frame_idx]) > 4:
			return frame_infos[frame_idx]

# each example is tuple (images, boxes, n_image, label, frame_idx, skip)
print('extracting examples')
all_examples = []
for label in labels:
	frame_infos, frame_matches = all_frame_data[label]

	for i, frame_idx in enumerate(frame_infos.keys()):
		print('...', label, i, len(frame_infos))

		match_lengths = [skip*match_len for match_len in MATCH_LENGTHS]

		infos = [frame_infos.get(frame_idx+l*skip, None) for l in range(model.SEQ_LEN)]
		if any([(info is None or len(info) == 0) for info in infos]):
			continue
		elif any([frame_idx not in frame_matches[match_len] for match_len in match_lengths]):
			continue

		neg_info = get_random_info(label)

		matches = [frame_matches[match_len][frame_idx] for match_len in match_lengths]
		images, boxes, detections, counts, mask = get_stuff(infos + [neg_info], matches)
		all_examples.append((
			images, boxes, counts, mask,
			label, frame_idx, detections,
		))

random.shuffle(all_examples)
random.shuffle(labels)
num_val = len(labels)//10+1
val_labels = set(labels[0:num_val])
val_examples = [example for example in all_examples if example[4] in val_labels]
if len(val_examples) > 1024:
	val_examples = random.sample(val_examples, 1024)
train_examples = [example for example in all_examples if example[4] not in val_labels and min(example[2][:-1]) >= 4]

# train_mode is 'imsp-longim' or 'imsp-finesp'
def train(train_mode):
	print('[{}] initializing model'.format(train_mode))
	m = model.Model()
	session = tf.Session()
	if train_mode == 'imsp-longim':
		session.run(m.init_op)
	else:
		m.saver.restore(session, model_path)

	print('[{}] begin training'.format(train_mode))
	best_loss = None
	consecutive_bad = 0
	for epoch in range(9999):
		start_time = time.time()
		train_losses = []
		for _ in range(2048//model.BATCH_SIZE):
			if train_mode == 'imsp-finesp':
				match_len = max(MATCH_LENGTHS)
			else:
				match_len = random.choice(MATCH_LENGTHS)

			batch = []
			for example in random.sample(train_examples, model.BATCH_SIZE):
				imlists = example[0][0:match_len+1] + [example[0][model.SEQ_LEN]]
				boxlists = example[1][0:match_len+1] + [example[1][model.SEQ_LEN]]
				counts = example[2][0:match_len+1] + [example[2][model.SEQ_LEN]]
				mask = example[3][MATCH_LENGTHS.index(match_len)]
				batch.append((imlists, boxlists, counts, mask))

			imlists = [imlist for example in batch for imlist in example[0]]
			boxlists = [boxlist for example in batch for boxlist in example[1]]
			counts = [[] for _ in range(len(batch))]
			for i, example in enumerate(batch):
				counts[i] = example[2][0:match_len+1]
				while len(counts[i]) < model.SEQ_LEN:
					counts[i].append(0)
				counts[i].append(example[2][-1])

			images = [im for imlist in imlists for im in imlist]
			boxes = [box for boxlist in boxlists for box in boxlist]

			masks = numpy.concatenate([example[3] for example in batch], axis=0)
			feed_dict = {
				m.raw_images: images,
				m.input_boxes: boxes,
				m.n_image: counts,
				m.input_masks: masks,
				m.match_length: match_len,
				m.is_training: True,
				m.learning_rate: 1e-4,
			}
			if train_mode == 'imsp-longim':
				_, loss = session.run([m.longim_optimizer, m.longim_loss], feed_dict=feed_dict)
			elif train_mode == 'imsp-finesp':
				_, loss = session.run([m.finesp_optimizer, m.finesp_loss], feed_dict=feed_dict)
			train_losses.append(loss)
		train_loss = numpy.mean(train_losses)
		train_time = time.time()

		val_losses = []
		for i in range(0, len(val_examples), model.BATCH_SIZE):
			batch = val_examples[i:i+model.BATCH_SIZE]
			images = [im for example in batch for imlist in example[0] for im in imlist]
			boxes = [box for example in batch for boxlist in example[1] for box in boxlist]
			counts = [example[2] for example in batch]
			masks = numpy.concatenate([example[3][-1] for example in batch], axis=0)
			feed_dict = {
				m.raw_images: images,
				m.input_boxes: boxes,
				m.n_image: counts,
				m.input_masks: masks,
				m.match_length: model.SEQ_LEN-1,
				m.is_training: False,
			}
			if train_mode == 'imsp-longim':
				loss = session.run(m.longim_loss, feed_dict=feed_dict)
			elif train_mode == 'imsp-finesp':
				loss = session.run(m.finesp_loss, feed_dict=feed_dict)
			val_losses.append(loss)

		val_loss = numpy.mean(val_losses)
		val_time = time.time()

		print('[{}] iteration {}: train_time={}, val_time={}, train_loss={}, val_loss={}/{}'.format(train_mode, epoch, int(train_time - start_time), int(val_time - train_time), train_loss, val_loss, best_loss))

		if best_loss is None or val_loss < best_loss:
			best_loss = val_loss
			m.saver.save(session, model_path)
			consecutive_bad = 0
		else:
			consecutive_bad += 1

		if consecutive_bad >= 10:
			break

	session.close()

train('imsp-longim')
train('imsp-finesp')

'''
from discoverlib import geom

def update_image(im, detections1, detections2, outputs):
	out = numpy.zeros(im.shape, dtype='uint8')
	out[:, :, :] = im

	m = {}
	for i in range(len(detections1)):
		for j in range(len(detections2)+1):
			output = outputs[i, j]
			if i not in m or output > m[i][0]:
				if j == len(detections2):
					m[i] = (output, detections1[i], detections1[i])
				else:
					m[i] = (output, detections1[i], detections2[j])

	for _, (_, p1, p2) in m.items():
		start = geom.Point(p1[1]*NORM, p1[0]*NORM)
		end = geom.Point(p2[1]*NORM, p2[0]*NORM)
		for p in geom.draw_line(start, end, geom.Point(im.shape[0], im.shape[1])):
			out[p.x-2:p.x+2, p.y-2:p.y+2, :] = [255, 0, 0]
	return out

def draw_detections(im, detections):
	out = numpy.zeros(im.shape, dtype='uint8')
	out[:, :, :] = im
	for p in detections:
		p = geom.Point(p[1]*NORM, p[0]*NORM)
		out[p.x-4:p.x+4, p.y-4:p.y+4, :] = [255, 255, 0]
	return out

def test():
	val_frames = {}
	for label in val_labels:
		video_fname = export_path + label + '_0.mp4'
		vreader = skvideo.io.vreader(video_fname)
		for frame_idx, im in enumerate(vreader):
			val_frames[(label, frame_idx)] = numpy.copy(im)

	for i in range(0, 64, model.BATCH_SIZE):
		batch = val_examples[i:i+model.BATCH_SIZE]
		images = [im for example in batch for imlist in example[0] for im in imlist]
		boxes = [box for example in batch for boxlist in example[1] for box in boxlist]
		counts = [example[2] for example in batch]
		masks = numpy.concatenate([example[3][-1] for example in batch], axis=0)
		feed_dict = {
			m.raw_images: images,
			m.input_boxes: boxes,
			m.n_image: counts,
			m.is_training: False,
			m.input_masks: masks,
			m.match_length: MATCH_LENGTHS[-1],
		}
		extra_mats, proc_masks, longim_losses = session.run([m.extra_mats, m.masks, m.longim_losses], feed_dict=feed_dict)
		for j, example in enumerate(batch):
			last_idx = model.SEQ_LEN-1
			images, boxes, counts, mask, label, frame_idx, detections = example
			probs2 = extra_mats[j][-1]
			im1 = val_frames[(label, frame_idx)]
			im3 = val_frames[(label, frame_idx+last_idx*skip)]
			skimage.io.imsave('/home/ubuntu/vis/{}_gt.jpg'.format(i+j), update_image(im1, detections[0], detections[last_idx], proc_masks[j]))
			skimage.io.imsave('/home/ubuntu/vis/{}_13_out2.jpg'.format(i+j), update_image(im1, detections[0], detections[last_idx], probs2))
			skimage.io.imsave('/home/ubuntu/vis/{}_13_b.jpg'.format(i+j), draw_detections(im3, detections[last_idx]))
'''
