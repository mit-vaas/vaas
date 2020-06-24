import model

import json
import keras
import numpy
import os, os.path
import skimage.io
import sys
import tensorflow as tf

export_path = sys.argv[1]
model_path = sys.argv[2]
num_classes = int(sys.argv[3])

onehots = []
for i in range(num_classes):
    onehot = numpy.zeros((num_classes,), dtype='float32')
    onehot[i] = 1
    onehots.append(onehot)

# read training data
X = []
Y = []
for fname in os.listdir(export_path):
    if not fname.endswith('_0.jpg'):
        continue
    label = fname.split('_0.jpg')[0]

    im_fname = os.path.join(export_path, label + '_0.jpg')
    im = skimage.io.imread(im_fname)
    X.append(im)

    cls_fname = os.path.join(export_path, label + '_1.json')
    with open(cls_fname, 'r') as f:
        cls = json.load(f)[0]
    Y.append(onehots[cls])
X = numpy.stack(X, axis=0)
Y = numpy.stack(Y, axis=0)

# train model
m = model.get_model(num_classes)
cb_checkpoint = keras.callbacks.ModelCheckpoint(
    filepath=model_path,
    save_weights_only=True,
    monitor='val_loss',
    mode='max',
    save_best_only=True
)
cb_stop = keras.callbacks.EarlyStopping(
    monitor="val_loss",
    patience=10,
)
m.fit(X, Y, epochs=1000, batch_size=1, validation_split=0.1, callbacks=[cb_checkpoint, cb_stop])
