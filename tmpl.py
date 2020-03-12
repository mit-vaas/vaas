import json
import math
import os
import os.path
import sys

def eprint(s):
	sys.stderr.write(str(s) + "\n")

labels = os.listdir('/bdd/frames/')

def get_center(detection):
	return ((detection['left'] + detection['right']) / 2, (detection['top'] + detection['bottom']) / 2)

def distance(p1, p2):
	dx = p2[0] - p1[0]
	dy = p2[1] - p1[1]
	return math.sqrt(dx*dx + dy*dy)

def contains(bbox, p):
	return p[0] >= bbox[0] and p[0] <= bbox[2] and p[1] >= bbox[1] and p[1] <= bbox[3]

def get_pred_time(track, idx, frames):
	for i in xrange(idx-1, -1, -1):
		if track[idx]['frame_idx'] - track[i]['frame_idx'] > frames:
			return i
	return None

def get_succ_time(track, idx, frames):
	for i in xrange(idx+1, len(track)):
		if track[i]['frame_idx'] - track[idx]['frame_idx'] > frames:
			return i
	return None

# def f(track): ...
[CODE]

for line is sys.stdin:
	track = json.loads(line)[0]
	out_tracks = f(track)
	print json.dumps(out_tracks)
