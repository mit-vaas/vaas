import keras.backend, keras.layers, keras.models
import numpy
import tensorflow as tf

def get_model(num_classes):
	model = keras.models.Sequential()
	model.add(keras.layers.Lambda(lambda im: im/255.0, input_shape=(None, None, 3)))
	for i in range(4):
		features = min(2**(5+i), 128)
		model.add(keras.layers.Conv2D(features, (4, 4), strides=2, activation='relu', padding='valid'))
		model.add(keras.layers.BatchNormalization())
	model.add(keras.layers.Conv2D(128, (4, 4), strides=2, activation='relu', padding='valid'))
	model.add(keras.layers.Lambda(lambda im: tf.math.reduce_max(im, axis=[1, 2])))
	model.add(keras.layers.Dense(num_classes))
	model.add(keras.layers.Activation('softmax'))
	model.compile(loss='categorical_crossentropy', optimizer='adam', metrics=['accuracy'])
	return model
