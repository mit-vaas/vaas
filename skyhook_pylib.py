import io
import json
import math
import numpy
import os
import os.path
import skimage.io
import struct
import sys

def eprint(s):
	sys.stderr.write(str(s) + "\n")
	sys.stderr.flush()

def get_center(detection):
	return ((detection['left'] + detection['right']) // 2, (detection['top'] + detection['bottom']) // 2)

def distance(p1, p2):
	dx = p2[0] - p1[0]
	dy = p2[1] - p1[1]
	return math.sqrt(dx*dx + dy*dy)

def contains(bbox, p):
	return p[0] >= bbox[0] and p[0] <= bbox[2] and p[1] >= bbox[1] and p[1] <= bbox[3]

def get_pred_time(track, idx, frames):
	for i in range(idx-1, -1, -1):
		if track[idx]['frame_idx'] - track[i]['frame_idx'] > frames:
			return i
	return None

def get_succ_time(track, idx, frames):
	for i in range(idx+1, len(track)):
		if track[i]['frame_idx'] - track[idx]['frame_idx'] > frames:
			return i
	return None

def get_tracks(detections):
	track_dict = {}
	for frame_idx in range(len(detections)):
		if detections[frame_idx].get('Detections', None) is None:
			continue
		for detection in detections[frame_idx]['Detections']:
			detection['frame_idx'] = frame_idx
			track_id = detection['track_id']
			if track_id not in track_dict:
				track_dict[track_id] = []
			track_dict[track_id].append(detection)
	return track_dict.values()

def tracks_to_detections(tracks, orig_detections):
	detections = [{'Detections': [], 'CanvasDims': d['CanvasDims']} for d in orig_detections]
	for track in tracks:
		for d in track:
			detections[d['frame_idx']]['Detections'].append(d)
	return detections

def per_frame_decorate(f):
	def wrap(*args):
		job_desc = args[0]
		if job_desc['type'] != 'job':
			return
		args = args[1:]
		outputs = []
		for i in range(len(args[0])):
			inputs = [arg[i] for arg in args]
			output = f(*inputs)
			outputs.append(output)
		if meta['Type'] == 'video':
			outputs = numpy.stack(outputs)
		output_packet(job_desc['slice_idx'], job_desc['range'], outputs)
	return wrap

def all_decorate(f):
	def wrap(*args):
		job_desc = args[0]
		all_inputs = job_desc['state']
		if job_desc['type'] == 'job':
			args = args[1:]
			if all_inputs is None:
				all_inputs = [[arg] for arg in args]
			else:
				for i, arg in enumerate(args):
					all_inputs[i].append(arg)
			return all_inputs
		elif job_desc['type'] == 'finish':
			for i, l in enumerate(all_inputs):
				if isinstance(l[0], list):
					all_inputs[i] = [x for arg in l for x in arg]
				else:
					all_inputs[i] = numpy.concatenate(l, axis=0)
			outputs = f(*all_inputs)
			output_packet(job_desc['slice_idx'], (0, len(all_inputs[0])), outputs)
	return wrap

stdin = None
stdout = None
meta = None

def input_packet():
	buf = stdin.read(5)
	if not buf:
		return None
	(l,) = struct.unpack('>I', buf[0:4])
	encoded_data = stdin.read(l)
	if buf[4:5] == b'j':
		return json.loads(encoded_data.decode('utf-8'))
	elif buf[4:5] == b'v':
		nframes, height, width, channels = struct.unpack('>IIII', encoded_data[0:16])
		return numpy.frombuffer(encoded_data, dtype='uint8', offset=16).reshape((nframes, height, width, channels))
	else:
		raise Exception('invalid packet type {}'.format(buf[4]))

def output_packet(slice_idx, frame_range, data):
	if meta['Type'] == 'video':
		encoded_data = struct.pack('>IIII', data.shape[0], data.shape[1], data.shape[2], data.shape[3]) + data.tobytes()
	elif meta['Type'] == 'imlist':
		encoded_data = struct.pack('>I', len(data))
		for imlist in data:
			encoded_data += struct.pack('>I', len(imlist))
			for im in imlist:
				buf = io.BytesIO()
				skimage.io.imsave(buf, im, 'imageio', format='jpeg')
				bin = buf.getvalue()
				encoded_data += struct.pack('>I', len(bin))
				encoded_data += bin
	else:
		encoded_data = json.dumps(data).encode('utf-8')
	stdout.write(struct.pack('>IIII', slice_idx, frame_range[0], frame_range[1], len(encoded_data)))
	stdout.write(encoded_data)
	stdout.flush()

def run(callback_func):
	global stdin, stdout, meta

	if sys.version_info[0] >= 3:
		stdin = sys.stdin.detach()
		stdout = sys.stdout.buffer
	else:
		stdin = sys.stdin
		stdout = sys.stdout
	meta = input_packet()

	states = {}
	while True:
		packet = input_packet()
		if packet is None:
			break
		if packet['Type'] == 'init':
			states[packet['ID']] = None
		elif packet['Type'] == 'job':
			# job packet
			slice_idx = packet['SliceIdx']
			inputs = [{
				'type': 'job',
				'range': packet['Range'],
				'slice_idx': slice_idx,
				'state': states[slice_idx],
			}]
			for _ in range(meta['Parents']):
				inputs.append(input_packet())
			states[slice_idx] = callback_func(*inputs)
		elif packet['Type'] == 'finish':
			inputs = [{
				'type': 'finish',
				'slice_idx': packet['ID'],
				'state': states[packet['ID']],
			}]
			inputs.extend([None]*meta['Parents'])
			callback_func(*inputs)
			del states[packet['ID']]
			stdout.write(struct.pack('>IIII', packet['ID'], 0, 0, 0))
			stdout.flush()
