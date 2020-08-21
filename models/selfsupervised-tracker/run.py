import model
model.BATCH_SIZE = 1
model.SEQ_LEN = 2

import json
import numpy
import math
import os
import skimage.io
import sys
import tensorflow as tf
import time

# import skyhook_pylib from ../../
# we use './' since we assume working directory is the skyhook root directory
sys.path.append('./')
import skyhook_pylib as lib

model_path = sys.argv[1]

MAX_AGE = 10
NORM = 1000.0
CROP_SIZE = 64

lib.eprint('initializing model')
m = model.Model()
config = tf.ConfigProto()
config.gpu_options.allow_growth = True
session = tf.Session(config=config)
m.saver.restore(session, model_path)

def get_loc(detection):
	cx = (detection['left'] + detection['right']) / 2
	cy = (detection['top'] + detection['bottom']) / 2
	cx = float(cx) / NORM
	cy = float(cy) / NORM
	return cx, cy

def get_info(im, detections):
	info = []
	for d in detections:
		if d['left'] < 0:
			d['left'] = 0
		if d['top'] < 0:
			d['top'] = 0
		if d['right'] > im.shape[1]:
			d['right'] = im.shape[1]
		if d['bottom'] > im.shape[0]:
			d['bottom'] = im.shape[0]
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
		info.append((d, fix_crop))
	return info

def get_stuff(infos):
	def per_info(info):
		images = []
		boxes = []
		for (detection, crop) in info:
			images.append(crop)
			cx, cy = get_loc(detection)
			boxes.append([cx, cy, detection['width'], detection['height']])
		detections = [get_loc(detection) for detection, _ in info]
		return images, boxes, detections, len(info)

	all_images = []
	all_boxes = []
	all_detections = []
	all_counts = []
	for info in infos:
		images, boxes, detections, count = per_info(info)
		all_images.extend(images)
		all_boxes.extend(boxes)
		all_detections.append(detections)
		all_counts.append(count)

	return all_images, all_boxes, all_detections, all_counts

def softmax(X, theta = 1.0, axis = None):
	y = numpy.atleast_2d(X)
	if axis is None:
	    axis = next(j[0] for j in enumerate(y.shape) if j[1] > 1)
	y = y * float(theta)
	y = y - numpy.expand_dims(numpy.max(y, axis = axis), axis)
	y = numpy.exp(y)
	ax_sum = numpy.expand_dims(numpy.sum(y, axis = axis), axis)
	p = y / ax_sum
	if len(X.shape) == 1: p = p.flatten()
	return p

def skyhook_callback(job_desc, frames, detection_data):
	if job_desc['type'] != 'job':
		return

	# state is tuple (active_objects, track_counter, previous info)
	if job_desc['state']:
		active_objects, track_counter, prev_info = job_desc['state']
	else:
		active_objects = None
		track_counter = 0
		prev_info = None

	new_data = []

	for frame_idx in range(len(frames)):
		im = frames[frame_idx]
		detections = detection_data[frame_idx]

		if not detections:
			active_objects = None
			prev_info = None
			continue

		cur_info = get_info(im, detections)
		if len(cur_info) == 0:
			active_objects = None
			prev_info = None
			continue
		images2, boxes2, _, counts2 = get_stuff([cur_info])

		while len(new_data) <= frame_idx:
			new_data.append([])
		new_data[frame_idx] = [d for (d, _) in cur_info]

		if prev_info is None or len(prev_info) == 0:
			# we have detections at frame_idx but not frame_idx-1
			# since we can't match with previous frame, here we
			# assign them new track IDs
			active_objects = []
			for idx in range(len(cur_info)):
				active_objects.append((
					track_counter,
					idx,
					numpy.zeros((64,), dtype='float32'),
					0,
					[images2[idx]],
				))
				new_data[frame_idx][idx]['track_id'] = track_counter
				track_counter += 1

			prev_info = cur_info
			continue

		images1, boxes1, _, counts1 = get_stuff([prev_info])

		# flatten the active objects since each object may have multiple images
		flat_images = []
		flat_boxes = []
		flat_hidden = []
		active_indices = {}
		for i, obj in enumerate(active_objects):
			active_indices[i] = []
			for j in [1, 2, 4, 8, 16]:
				if len(obj[4]) < j:
					continue
				# use image from stored history, but use current box
				active_indices[i].append(len(flat_images))
				flat_images.append(obj[4][-j])
				if obj[1] < len(prev_info):
					flat_boxes.append(boxes1[obj[1]])
				else:
					flat_boxes.append(numpy.zeros((4,), dtype='float32'))
				flat_hidden.append(obj[2])

		feed_dict = {
			m.raw_images: flat_images + images2,
			m.input_boxes: flat_boxes + boxes2,
			m.n_image: [[len(flat_images), len(images2), 0]],
			m.is_training: False,
			m.infer_sel: range(len(flat_images)),
			m.infer_hidden: flat_hidden,
		}

		longim_logits, finesp_logits, pre_cur_hidden = session.run([m.out_logits_longim, m.out_logits_finesp, m.out_hidden], feed_dict=feed_dict)
		longim_out_logits = numpy.zeros((len(active_objects), len(cur_info)+1), dtype='float32')
		finesp_out_logits = numpy.zeros((len(active_objects), len(cur_info)+1), dtype='float32')
		cur_hidden = numpy.zeros((len(active_objects), len(cur_info)+1, 64), dtype='float32')
		for i, obj in enumerate(active_objects):
			longim_out_logits[i, 0:len(cur_info)] = longim_logits[active_indices[i], 0:len(cur_info)].mean(axis=0)
			longim_out_logits[i, len(cur_info)] = longim_logits[active_indices[i], len(cur_info)].min()
			finesp_out_logits[i, 0:len(cur_info)] = finesp_logits[active_indices[i], 0:len(cur_info)].mean(axis=0)
			finesp_out_logits[i, len(cur_info)] = finesp_logits[active_indices[i], len(cur_info)].min()
			cur_hidden[i, :, :] = pre_cur_hidden[active_indices[i][0], :, :]
		longim_mat = numpy.minimum(softmax(longim_out_logits, axis=0), softmax(longim_out_logits, axis=1))
		finesp_mat = numpy.minimum(softmax(finesp_out_logits, axis=0), softmax(finesp_out_logits, axis=1))
		outputs = numpy.minimum(longim_mat, finesp_mat)

		# vote on best next frame: idx1->(output,idx2)
		votes = {}
		for i in range(len(active_objects)):
			for j in range(len(cur_info)+1):
				output = outputs[i, j]
				if j != len(cur_info) and (longim_out_logits[i, j] < 0 or finesp_out_logits[i, j] < 0):
					output = -100.0
				if i not in votes or output > votes[i][0]:
					if j < len(cur_info):
						votes[i] = (output, j)
					else:
						votes[i] = (output, None)
		# group by receiver and vote on max idx2->idx1 to eliminate duplicates
		votes2 = {}
		for idx1, t in votes.items():
			output, idx2 = t
			if idx2 is not None and (idx2 not in votes2 or output > votes2[idx2][0]):
				votes2[idx2] = (output, idx1)
		forward_matches = {idx1: idx2 for (idx2, (_, idx1)) in votes2.items()}

		new_objects = []
		used_idx2s = set()
		for idx1, obj in enumerate(active_objects):
			if idx1 in forward_matches:
				idx2 = forward_matches[idx1]
				new_objects.append((
					obj[0],
					idx2,
					cur_hidden[idx1, idx2, :],
					0,
					obj[4] + [images2[idx2]],
				))
				used_idx2s.add(idx2)
				new_data[frame_idx][idx2]['track_id'] = obj[0]
			elif obj[3] < MAX_AGE:
				idx2 = votes[idx1][1]
				if idx2 is None or True:
					idx2 = len(cur_info)
				new_objects.append((
					obj[0],
					idx2,
					cur_hidden[idx1, idx2, :],
					obj[3]+1,
					obj[4],
				))

		# assign new track IDs to detections in the current frame that
		# didn't match with any detection in the previous frame
		for idx2 in range(len(cur_info)):
			if idx2 in used_idx2s:
				continue
			new_objects.append((
				track_counter,
				idx2,
				numpy.zeros((64,), dtype='float32'),
				0,
				[images2[idx2]],
			))
			new_data[frame_idx][idx2]['track_id'] = track_counter
			track_counter += 1

		active_objects = new_objects
		prev_info = cur_info

	lib.output_packet(job_desc['slice_idx'], job_desc['range'], new_data)
	return (active_objects, track_counter, prev_info)

lib.run(skyhook_callback)
