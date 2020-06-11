import json
import math
import numpy
import os
import os.path
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
		if detections[frame_idx] is None:
			continue
		for detection in detections[frame_idx]:
			detection['frame_idx'] = frame_idx
			track_id = detection['track_id']
			if track_id not in track_dict:
				track_dict[track_id] = []
			track_dict[track_id].append(detection)
	return track_dict.values()

def tracks_to_detections(tracks, n):
	detections = [[] for _ in range(n)]
	for track in tracks:
		for d in track:
			detections[d['frame_idx']].append(d)
	return detections

def per_frame_decorate(f):
	def wrap(*args):
		job_desc = args[0]
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
		args = args[1:]
		if all_inputs is None:
			all_inputs = [[arg] for arg in args]
		else:
			for i, arg in enumerate(args):
				all_inputs[i].append(arg)
		if not job_desc['is_last']:
			return all_inputs
		for i, l in enumerate(all_inputs):
			if isinstance(l[0], list):
				all_inputs[i] = [x for arg in l for x in arg]
			else:
				all_inputs[i] = numpy.concatenate(l, axis=0)
		outputs = f(*all_inputs)
		output_packet(job_desc['slice_idx'], (0, job_desc['range'][1]), outputs)
	return wrap

# def f(context, parent1, parent2, ...): ...
[CODE]

stdin = sys.stdin.detach()

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
	if meta['Type'] == 'detection' or meta['Type'] == 'track':
		encoded_data = json.dumps(data).encode('utf-8')
	elif meta['Type'] == 'video':
		encoded_data = struct.pack('>IIII', data.shape[0], data.shape[1], data.shape[2], data.shape[3]) + data.tobytes()
	sys.stdout.buffer.write(struct.pack('>IIII', slice_idx, frame_range[0], frame_range[1], len(encoded_data)))
	sys.stdout.buffer.write(encoded_data)

meta = input_packet()
states = {}
while True:
	packet = input_packet()
	if packet is None:
		break
	if 'ID' in packet:
		# init packet
		states[packet['ID']] = None
	else:
		# job packet
		slice_idx = packet['SliceIdx']
		inputs = [{
			'range': packet['Range'],
			'is_last': packet['IsLast'],
			'slice_idx': slice_idx,
			'state': states[slice_idx],
		}]
		for _ in range(meta['Parents']):
			inputs.append(input_packet())
		states[slice_idx] = f(*inputs)
