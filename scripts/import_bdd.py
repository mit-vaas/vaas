import os
import sqlite3
import subprocess

conn = sqlite3.connect('skyhook.sqlite3')
c = conn.cursor()

for dirname in os.listdir('/mnt/bdd/bdd-selected-videos/frames-half/'):
	nframes = len([fname for fname in os.listdir('/mnt/bdd/bdd-selected-videos/frames-half/' + dirname) if fname.endswith('.jpg')])
	c.execute('INSERT INTO clips (video_id, nframes, width, height) VALUES (4, ?, 640, 360)', (nframes,))
	subprocess.call(['ln', '-s', '/mnt/bdd/bdd-selected-videos/frames-half/' + dirname, 'clips/4/{}'.format(c.lastrowid)])

conn.commit()
conn.close()
