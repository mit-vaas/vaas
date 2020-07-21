import model

import json
import keras
import numpy
import os, os.path
import random
import skimage.io, skimage.transform
import sys
import tensorflow as tf

export_path = sys.argv[1]
model_path = sys.argv[2]
num_classes = int(sys.argv[3])
max_width = int(sys.argv[4])
max_height = int(sys.argv[5])

onehots = []
for i in range(num_classes):
    onehot = numpy.zeros((num_classes,), dtype='float32')
    onehot[i] = 1
    onehots.append(onehot)

dims = model.get_dims(max_width, max_height)

# read training data
examples = []
for fname in os.listdir(export_path):
    if not fname.endswith('_0.jpg'):
        continue
    label = fname.split('_0.jpg')[0]
    print('load', label)

    im_fname = os.path.join(export_path, label + '_0.jpg')
    im = skimage.io.imread(im_fname)
    if im.shape[0] != max_height or im.shape[1] != max_width:
        im = skimage.transform.resize(im, [max_height, max_width], preserve_range=True).astype('uint8')

    cls_fname = os.path.join(export_path, label + '_1.json')
    with open(cls_fname, 'r') as f:
        cls = json.load(f)[0]

    examples.append((im, cls))

class MyGenerator(keras.utils.Sequence):
    def __init__(self, examples, batch_size=1):
        self.examples = examples
        self.batch_size = batch_size

    def __len__(self):
        return len(self.examples)//self.batch_size

    def __getitem__(self, idx):
        batch = self.examples[idx*self.batch_size:(idx+1)*self.batch_size]
        ims = numpy.array([t[0] for t in batch], dtype='uint8')
        weights = numpy.ones((self.batch_size, len(dims)), dtype='float32')
        for i in range(self.batch_size):
            r = random.randint(0, len(dims)-1)
            weights[i, 0:r] = 0
        y = numpy.zeros((self.batch_size, num_classes), dtype='float32')
        for i, (_, cls) in enumerate(batch):
            y[i, :] = onehots[cls]
        return [ims, weights], [y]*len(dims)

# train model
m = model.get_model(num_classes, dims)
cb_checkpoint = keras.callbacks.ModelCheckpoint(
    filepath=model_path,
    save_weights_only=True,
    monitor='val_loss',
    mode='min',
    save_best_only=True
)
cb_stop = keras.callbacks.EarlyStopping(
    monitor="val_loss",
    patience=10,
)
random.shuffle(examples)
num_val = len(examples)//10+1
val_examples = examples[0:num_val]
train_examples = examples[num_val:]
m.fit(
    MyGenerator(train_examples, batch_size=16),
    epochs=100,
    validation_data=MyGenerator(val_examples),
    callbacks=[cb_checkpoint, cb_stop]
)
