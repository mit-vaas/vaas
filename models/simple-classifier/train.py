import model

import numpy
import os, os.path
import skimage.io
import sys
import tensorflow as tf

export_path = sys.argv[1]
model_path = sys.argv[2]
num_classes = int(sys.argv[3])

im_path = os.path.join(export_path, '0')
cls_path = os.path.join(export_path, '1')

onehots = []
for i in range(num_classes):
    onehot = numpy.zeros((num_classes,), dtype='float32')
    onehot[i] = 1
    onehots.append(onehot)

# read training data
X = []
Y = []
for fname in os.listdir(im_path):
    label = fname.split('.')[0]

    im_fname = os.path.join(im_path, label + '.jpg')
    im = skimage.io.imread(fname)
    X.append(im)

    cls_fname = os.path.join(cls_path, label + '.json')
    with open(cls_fname, 'r') as f:
        cls = json.load(f)[0]
    Y.append(onehots[cls])

# train model
model = model.get_model(num_classes)
cb_checkpoint = tf.keras.callbacks.ModelCheckpoint(
    filepath=model_path,
    save_weights_only=True,
    monitor='val_acc',
    mode='max',
    save_best_only=True
)
cb_stop = tf.keras.callbacks.EarlyStopping(
    monitor="val_loss",
    patience=10,
)

model = common.get_model()
model.fit(X, Y, epochs=1000, batch_size=8, validation_split=0.1, callbacks=[cb_checkpoint, cb_stop])
