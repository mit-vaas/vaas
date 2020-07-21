import keras.backend, keras.layers, keras.models
import numpy
import tensorflow as tf

# Returns list of input dimensions given this max dimension.
def get_dims(max_width, max_height):
	w, h = max_width, max_height
	dims = []
	while min(w, h) >= 4:
		dims.append((w, h))
		w = (w-2)//2
		h = (h-2)//2
	return dims

def get_model(num_classes, dims):
	im_input = keras.layers.Input(shape=(dims[0][1], dims[0][0], 3), dtype='uint8')
	weight_input = keras.layers.Input(shape=(len(dims),), dtype='float32')

	names = []
	for dim in dims:
		names.append('{}x{}'.format(dim[0], dim[1]))

	# get input image at different resolutions
	fimages = []
	for dim in dims:
		if fimages:
			prev_im = fimages[-1]
		else:
			prev_im = keras.layers.Lambda(lambda im: tf.cast(im, tf.float32)/255.0)(im_input)
		fimage = keras.layers.Lambda(lambda im: tf.image.resize_images(im, [dim[1], dim[0]], method='nearest'))(prev_im)
		fimages.append(fimage)

	conv0 = keras.layers.Conv2D(
		32, (4, 4),
		strides=2, activation='relu', padding='valid',
		name='im_conv{}'.format(names[0])
	)(fimages[0])
	conv_outputs = [conv0]
	for i, fimage in enumerate(fimages[1:]):
		if False:
			im_conv = keras.layers.Conv2D(
				32, (4, 4),
				strides=2, activation='relu', padding='valid',
				name='im_conv{}'.format(names[i+1])
			)(fimage)
			feature_conv = keras.layers.Conv2D(
				32, (4, 4),
				strides=2, activation='relu', padding='valid',
				name='feature_conv{}'.format(names[i+1])
			)(conv_outputs[-1])
			feature_conv = keras.layers.Lambda(lambda l: l[0]*tf.reshape(l[1][:, i], [-1, 1, 1, 1]))([feature_conv, weight_input])
			conv = keras.layers.Lambda(lambda l: l[0]+l[1])([im_conv, feature_conv])
		if False:
			im_conv = keras.layers.Conv2D(
				32, (4, 4),
				strides=2, activation='relu', padding='valid',
				name='im_conv{}'.format(names[i+1])
			)(fimage)
			feature_conv = keras.layers.Conv2D(
				32, (4, 4),
				strides=2, activation='relu', padding='valid',
				name='feature_conv{}'.format(names[i+1])
			)(conv_outputs[-1])
			conv = keras.layers.Lambda(lambda l: (l[0]+l[1]*tf.reshape(l[2][:, i], [-1, 1, 1, 1]))/tf.reshape(1+l[2][:, i], [-1, 1, 1, 1]))([im_conv, feature_conv, weight_input])
		if True:
			if i == 0:
				conv_outputs[0] = keras.layers.Lambda(lambda x: tf.concat([
					x,
					tf.zeros(tf.shape(x), dtype='float32'),
				], axis=3))(conv_outputs[0])
			im_conv = keras.layers.Conv2D(
				32, (4, 4),
				strides=2, activation='relu', padding='valid',
				name='im_conv{}'.format(names[i+1])
			)(fimage)
			feature_conv = keras.layers.Conv2D(
				32, (4, 4),
				strides=2, activation='relu', padding='valid',
				name='feature_conv{}'.format(names[i+1])
			)(conv_outputs[-1])
			feature_conv = keras.layers.Lambda(lambda l: l[0]*tf.reshape(l[1][:, i], [-1, 1, 1, 1]))([feature_conv, weight_input])
			conv = keras.layers.Lambda(lambda l: tf.concat(l, axis=3))([im_conv, feature_conv])
		if False:
			if i == 0:
				conv_outputs[0] = keras.layers.Lambda(lambda x: tf.concat([
					2*x,
					tf.zeros(tf.shape(x), dtype='float32'),
				], axis=3))(conv_outputs[0])
			im_conv = keras.layers.Conv2D(
				32, (4, 4),
				strides=2, activation='relu', padding='valid',
				name='im_conv{}'.format(names[i+1])
			)(fimage)
			feature_conv = keras.layers.Conv2D(
				32, (4, 4),
				strides=2, activation='relu', padding='valid',
				name='feature_conv{}'.format(names[i+1])
			)(conv_outputs[-1])
			feature_conv = keras.layers.Lambda(lambda l: l[0]*tf.reshape(l[1][:, i], [-1, 1, 1, 1]))([feature_conv, weight_input])
			im_conv = keras.layers.Lambda(lambda l: l[0]*tf.reshape(2-l[1][:, i], [-1, 1, 1, 1]))([im_conv, weight_input])
			conv = keras.layers.Lambda(lambda l: tf.concat(l, axis=3))([im_conv, feature_conv])

		conv_outputs.append(conv)

	cls_outputs = []
	losses = {}
	loss_weights = {}
	for i, conv in enumerate(conv_outputs):
		flat = keras.layers.Lambda(lambda im: tf.math.reduce_mean(im, axis=[1, 2]))(conv)
		out = keras.layers.Dense(num_classes, name='flat{}'.format(names[i]))(flat)

		# bit of a hack
		# we multiply `out` by the input weight so that the associated loss is constant
		# this way, this adds 0 to the gradient if the weight is 0
		out = keras.layers.Lambda(lambda l: l[0]*tf.reshape(l[1][:, i], [-1, 1]))([out, weight_input])

		name = 'out{}'.format(names[i])
		out = keras.layers.Activation('softmax', name=name)(out)
		cls_outputs.append(out)
		losses[name] = 'categorical_crossentropy'
		loss_weights[name] = 1.0

	model = keras.models.Model(inputs=[im_input, weight_input], outputs=cls_outputs)
	model.compile(optimizer='adam', loss=losses, loss_weights=loss_weights, metrics=['accuracy'])
	return model
