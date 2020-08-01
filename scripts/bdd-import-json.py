import json
import os
import sqlite3
import sys

video_series_id = int(sys.argv[1])
json_series_id = int(sys.argv[2])

conn = sqlite3.connect('skyhook.sqlite3')
c = conn.cursor()

c.execute("SELECT segments.id, segments.name, segments.frames FROM segments, items WHERE items.series_id = ? AND segments.id = items.segment_id", (video_series_id,))
rows = c.fetchall()
for segment_id, name, nframes in rows:
	print(name)
	fname = '/data2/bdd/bdd100k/info/100k/train/' + name.replace('.mov', '.json')
	with open(fname, 'r') as f:
		try:
			info = json.load(f)
		except Exception as e:
			print(e)
			info = {}
	locations = info.get('locations', None)
	if not locations:
		locations = [{}]
	cur_idx = 0
	strs = []
	for frame_idx in range(nframes):
		while cur_idx+1 < len(locations):
			interp_frame = (locations[cur_idx]['timestamp'] - info['startTime']) * nframes // (info['endTime'] - info['startTime'])
			if interp_frame < frame_idx:
				cur_idx += 1
				continue
			break
		strs.append(json.dumps(locations[cur_idx]))
	c.execute("INSERT INTO items (segment_id, series_id, start, end, format) VALUES (?, ?, 0, ?, 'json')", (segment_id, json_series_id, nframes))
	item_id = int(c.lastrowid)
	with open('items/{}/{}.json'.format(json_series_id, item_id), 'w') as f:
		json.dump(strs, f)

conn.commit()
conn.close()
