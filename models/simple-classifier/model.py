import keras.backend, keras.layers, keras.models
import numpy
import tensorflow as tf

def get_model(num_classes, width, height):
	# determine number of layers based on width and height
	w, h = width//2, height//2
	nlayers = 1
	while min(w, h) >= 2:
		nlayers += 1
		w = w//2
		h = h//2

	model = keras.models.Sequential()
	model.add(keras.layers.Lambda(lambda im: im/255.0, input_shape=(None, None, 3)))
	for i in range(nlayers):
		features = min(2**(5+i), 128)
		model.add(keras.layers.Conv2D(features, (4, 4), strides=2, activation='relu', padding='same'))
	model.add(keras.layers.Lambda(lambda im: tf.math.reduce_max(im, axis=[1, 2])))
	model.add(keras.layers.Dense(num_classes))
	model.add(keras.layers.Activation('softmax'))
	model.compile(loss='categorical_crossentropy', optimizer='adam', metrics=['accuracy'])
	return model
