import numpy
import tensorflow as tf
import os
import os.path
import random
import math
import time

BATCH_SIZE = 1
SEQ_LEN = 17
KERNEL_SIZE = 3

class Model:
	def _conv_layer(self, name, input_var, stride, in_channels, out_channels, options = {}):
		activation = options.get('activation', 'relu')
		dropout = options.get('dropout', None)
		padding = options.get('padding', 'SAME')
		batchnorm = options.get('batchnorm', False)
		transpose = options.get('transpose', False)

		with tf.variable_scope(name) as scope:
			if not transpose:
				filter_shape = [KERNEL_SIZE, KERNEL_SIZE, in_channels, out_channels]
			else:
				filter_shape = [KERNEL_SIZE, KERNEL_SIZE, out_channels, in_channels]
			kernel = tf.get_variable(
				'weights',
				shape=filter_shape,
				initializer=tf.truncated_normal_initializer(stddev=math.sqrt(2.0 / KERNEL_SIZE / KERNEL_SIZE / in_channels)),
				dtype=tf.float32
			)
			biases = tf.get_variable(
				'biases',
				shape=[out_channels],
				initializer=tf.constant_initializer(0.0),
				dtype=tf.float32
			)
			if not transpose:
				output = tf.nn.bias_add(
					tf.nn.conv2d(
						input_var,
						kernel,
						[1, stride, stride, 1],
						padding=padding
					),
					biases
				)
			else:
				batch = tf.shape(input_var)[0]
				side = tf.shape(input_var)[1]
				output = tf.nn.bias_add(
					tf.nn.conv2d_transpose(
						input_var,
						kernel,
						[batch, side * stride, side * stride, out_channels],
						[1, stride, stride, 1],
						padding=padding
					),
					biases
				)
			if batchnorm:
				output = tf.contrib.layers.batch_norm(output, center=True, scale=True, is_training=self.is_training, decay=0.99)
			if dropout is not None:
				output = tf.nn.dropout(output, keep_prob=1-dropout)

			if activation == 'relu':
				return tf.nn.relu(output, name=scope.name)
			elif activation == 'sigmoid':
				return tf.nn.sigmoid(output, name=scope.name)
			elif activation == 'none':
				return output
			else:
				raise Exception('invalid activation {} specified'.format(activation))

	def _fc_layer(self, name, input_var, input_size, output_size, options = {}):
		activation = options.get('activation', 'relu')
		dropout = options.get('dropout', None)
		batchnorm = options.get('batchnorm', False)

		with tf.variable_scope(name) as scope:
			weights = tf.get_variable(
				'weights',
				shape=[input_size, output_size],
				initializer=tf.truncated_normal_initializer(stddev=math.sqrt(2.0 / input_size)),
				dtype=tf.float32
			)
			biases = tf.get_variable(
				'biases',
				shape=[output_size],
				initializer=tf.constant_initializer(0.0),
				dtype=tf.float32
			)
			output = tf.matmul(input_var, weights) + biases
			if batchnorm:
				output = tf.contrib.layers.batch_norm(output, center=True, scale=True, is_training=self.is_training, decay=0.99)
			if dropout is not None:
				output = tf.nn.dropout(output, keep_prob=1-dropout)

			if activation == 'relu':
				return tf.nn.relu(output, name=scope.name)
			elif activation == 'sigmoid':
				return tf.nn.sigmoid(output, name=scope.name)
			elif activation == 'none':
				return output
			else:
				raise Exception('invalid activation {} specified'.format(activation))

	def __init__(self):
		tf.reset_default_graph()

		c_image = 64
		c_spatial = 4
		c_features = c_image+c_spatial
		c_rnn = 64

		self.is_training = tf.placeholder(tf.bool)
		self.raw_images = tf.placeholder(tf.uint8, [None, 64, 64, 3])
		self.input_images = tf.cast(self.raw_images, tf.float32)/255.0
		self.input_boxes = tf.placeholder(tf.float32, [None, 4])
		self.n_image = tf.placeholder(tf.int32, [BATCH_SIZE, SEQ_LEN+1])
		self.input_masks = tf.placeholder(tf.float32, [None])
		self.match_length = tf.placeholder(tf.int32)
		self.learning_rate = tf.placeholder(tf.float32)

		# for inference
		self.infer_sel = tf.placeholder(tf.int32, [None])
		self.infer_hidden = tf.placeholder(tf.float32, [None, c_rnn])

		last_idx = self.match_length

		# extract masks
		self.masks = []
		s = 0
		for batch in range(BATCH_SIZE):
			n_first = self.n_image[batch, 0]
			n_last = self.n_image[batch, last_idx]
			cur_count = n_first*(n_last+1)
			cur_mask = tf.reshape(self.input_masks[s:s+cur_count], [n_first, n_last+1])
			self.masks.append(cur_mask)
			s += cur_count

		# CNN for long-term image
		self.layer1 = self._conv_layer('layer1', self.input_images, 2, 3, 64) # -> 32x32x64
		self.layer2 = self._conv_layer('layer2', self.layer1, 2, 64, c_image) # -> 16x16x64
		self.layer3 = self._conv_layer('layer3', self.layer2, 2, c_image, c_image) # -> 8x8x64
		self.layer4 = self._conv_layer('layer4', self.layer3, 2, c_image, c_image) # -> 4x4x64
		self.layer5 = self._conv_layer('layer5', self.layer4, 2, c_image, c_image) # -> 2x2x64
		self.layer6 = self._conv_layer('layer6', self.layer5, 2, c_image, c_image, {'activation': 'none'})[:, 0, 0, :]

		self.features = [[] for _ in range(BATCH_SIZE)]
		s = 0
		for batch in range(BATCH_SIZE):
			for i in range(SEQ_LEN+1):
				cur_count = self.n_image[batch, i]
				cur_features = tf.concat([
					self.input_boxes[s:s+cur_count, :],
					self.layer6[s:s+cur_count, :],
				], axis=1)
				cur_features = tf.concat([
					cur_features,
					tf.zeros([1, c_features], dtype=tf.float32),
				], axis=0)
				self.features[batch].append(cur_features)
				s += cur_count

		# MATCHER
		# context is longim or finesp
		def matcher(pairs, context):
			with tf.variable_scope('matcher' + context, reuse=tf.AUTO_REUSE):
				im_pairs = tf.concat([pairs[:, 0:c_rnn], pairs[:, c_rnn+4:c_rnn+c_features], pairs[:, c_rnn+c_features+4:]], axis=1)
				spatial_pairs = tf.concat([pairs[:, 0:c_rnn+4], pairs[:, c_rnn+c_features:c_rnn+c_features+4]], axis=1)

				if context == 'longim':
					matcher1 = self._fc_layer('matcher1', im_pairs, c_rnn+2*c_image, 256)
					matcher2 = self._fc_layer('matcher2', matcher1, 256, 65, {'activation': 'none'})
					return matcher2
				elif context == 'finesp':
					matcher1 = self._fc_layer('matcher1', spatial_pairs, c_rnn+2*c_spatial, 256)
					matcher2 = self._fc_layer('matcher2', matcher1, 256, 128)
					matcher3 = self._fc_layer('matcher3', matcher2, 128, 128)
					matcher4 = self._fc_layer('matcher4', matcher3, 128, 1, {'activation': 'none'})

					matcher5 = self._fc_layer('matcher5', spatial_pairs, c_rnn+2*c_spatial, 256)
					matcher6 = self._fc_layer('matcher6', matcher1, 256, 128)
					matcher7 = self._fc_layer('matcher7', matcher2, 128, 128)
					matcher8 = self._fc_layer('matcher8', matcher3, 128, c_rnn, {'activation': 'none'})

					return tf.concat([matcher4, matcher8], axis=1)

		# logit replacing matching some detection with the zero (null/fake) detection
		self.no_match_logit = tf.get_variable('no_match_logit', shape=[1], initializer=tf.constant_initializer(0.0), dtype=tf.float32)

		def get_mat_hidden(n_prev, n_next, rnn_features, prev_features, next_features, context, incl_logits=False, do_neg=True):
			if do_neg:
				fake_next1 = n_prev
				fake_next2 = tf.minimum(n_next, self.n_image[0, SEQ_LEN])
				fake_next = fake_next1 + fake_next2
				n_next += fake_next
				neg_features1 = prev_features
				neg_features2 = tf.concat([
					next_features[0:fake_next2, 0:c_spatial],
					self.features[0][SEQ_LEN][0:fake_next2, c_spatial:],
				], axis=1)
				next_features = tf.concat([neg_features1, neg_features2, next_features], axis=0)

			cur_pairs = tf.concat([
				tf.tile(
					tf.reshape(rnn_features, [n_prev, 1, c_rnn]),
					[1, n_next+1, 1]
				),
				tf.tile(
					tf.reshape(prev_features, [n_prev, 1, c_features]),
					[1, n_next+1, 1]
				),
				tf.tile(
					tf.reshape(next_features, [1, n_next+1, c_features]),
					[n_prev, 1, 1]
				),
			], axis=2)

			cur_pairs = tf.reshape(cur_pairs, [n_prev*(n_next+1), c_rnn+2*c_features])
			cur_outputs = matcher(cur_pairs, context=context)
			cur_outputs = tf.reshape(cur_outputs, [n_prev, n_next+1, 1+c_rnn])

			cur_logits = cur_outputs[:, :, 0]
			cur_logits = tf.concat([
				cur_logits[:, :-1],
				tf.tile(tf.reshape(self.no_match_logit, [1, 1]), [n_prev, 1]),
			], axis=1)

			if do_neg:
				# need to eliminate logits that are connecting the same features
				# these are in the first n_prev x n_prev of the matrix
				elim_mat = tf.eye(num_rows=n_prev, num_columns=n_next+1)
				cur_logits = (cur_logits*(1-elim_mat)) - 50*elim_mat

			cur_mat = tf.math.minimum(
				tf.nn.softmax(cur_logits, axis=0),
				tf.nn.softmax(cur_logits, axis=1)
			)
			cur_hidden = cur_outputs[:, :, 1:]

			if do_neg:
				cur_logits = cur_logits[:, fake_next:]
				cur_mat = cur_mat[:, fake_next:]
				cur_hidden = cur_hidden[:, fake_next:, :]

			if incl_logits:
				return cur_mat, cur_hidden, cur_logits
			else:
				return cur_mat, cur_hidden

		def index_list(l, idx, out_shape):
			flatlist = []
			sums = [0]
			for t in l:
				flat = tf.reshape(t, [-1])
				flatlist.append(flat)
				sums.append(sums[-1] + tf.shape(flat)[0])
			flatlist = tf.concat(flatlist, axis=0)
			sums = tf.stack(sums, axis=0)
			output = flatlist[sums[idx]:sums[idx+1]]
			return tf.reshape(output, out_shape)

		def terminal_reweight(mat):
			mat_term = mat[:, -1]
			factor = tf.minimum(1.0/(tf.reduce_sum(mat_term)+1e-2), tf.cast(tf.shape(mat)[0], tf.float32))
			mat_term = mat_term * factor
			row_maxes = 1 - tf.reduce_sum(mat[:, :-1], axis=1)
			row_maxes = tf.maximum(row_maxes, 0)
			mat_term = tf.minimum(mat_term, row_maxes)
			return tf.concat([mat[:, :-1], tf.reshape(mat_term, [-1, 1])], axis=1)

		def get_recur_sel(mat):
			def f(mat):
				# take argmax along rows (over columns)
				# but only use it if it is higher value than other rows in same column
				row_argmax = numpy.argmax(mat, axis=1)
				col_argmax = numpy.argmax(mat, axis=0)
				out = row_argmax
				for i in range(out.shape[0]):
					if col_argmax[out[i]] != i:
						out[i] = mat.shape[1]-1
				return out.astype('int32')

			sel = tf.py_func(f, [mat], tf.int32, stateful=False)
			return sel

		def compute_loss(mat1, mat2, batch, apply_mask=True):
			if apply_mask:
				mask = self.masks[batch]
			else:
				mask = tf.ones(tf.shape(mat1), dtype=tf.float32)

			epsilon = 1e-8
			loss = -tf.reduce_mean(tf.log(tf.reduce_sum(terminal_reweight(mat1) * terminal_reweight(mat2) * mask, axis=1) + epsilon))
			return loss

		if SEQ_LEN < 4:
			# inference
			n_prev = tf.shape(self.infer_sel)[0]
			n_next = self.n_image[0, 1]
			rnn_features = self.infer_hidden
			prev_features = tf.gather(self.features[0][0], self.infer_sel, axis=0)
			next_features = self.features[0][1]

			self.out_mat_finesp, self.out_hidden, self.out_logits_finesp = get_mat_hidden(n_prev, n_next, rnn_features, prev_features, next_features, 'finesp', incl_logits=True, do_neg=False)
			self.out_mat_longim, _, self.out_logits_longim = get_mat_hidden(n_prev, n_next, tf.zeros(tf.shape(rnn_features), dtype=tf.float32), prev_features, next_features, 'longim', incl_logits=True, do_neg=False)
			self.out_mat = tf.minimum(self.out_mat_finesp, self.out_mat_longim)
			self.out_mat_reweight = terminal_reweight(self.out_mat)

		else:
			finesp_indices = []
			for i in range(SEQ_LEN-1):
				finesp_indices.append((i, i+1))

			# LONGIM
			self.extra_mats = [[] for _ in range(BATCH_SIZE)]
			self.extra_mats_finesp = [[] for _ in range(BATCH_SIZE)]
			for batch in range(BATCH_SIZE):
				n_prev = self.n_image[batch, 0]
				n_next = self.n_image[batch, self.match_length]
				rnn_features = tf.zeros((n_prev, 1, c_rnn), dtype=tf.float32)
				prev_features = self.features[batch][0][:-1, :]
				# next_features = self.features[batch][match_length]
				next_features = index_list(self.features[batch], self.match_length, [n_next+1, c_features])
				cur_mat, _ = get_mat_hidden(n_prev, n_next, rnn_features, prev_features, next_features, 'longim')
				self.extra_mats[batch].append(cur_mat)

				for i in range(SEQ_LEN-1):
					# for extra_mats_finesp we always have SEQ_LEN inputs
					n_prev = self.n_image[batch, 0]
					n_next = self.n_image[batch, i+1]
					rnn_features = tf.zeros((n_prev, 1, c_rnn), dtype=tf.float32)
					prev_features = self.features[batch][0][:-1, :]
					next_features = self.features[batch][i+1]
					cur_mat, _ = get_mat_hidden(n_prev, n_next, rnn_features, prev_features, next_features, 'longim', do_neg=False)
					self.extra_mats_finesp[batch].append(cur_mat)

			# FINESP (note: this can't be executed with variable matchlen, at least for now)
			self.finesp_mats = [[] for _ in range(BATCH_SIZE)]
			self.finesp_hiddens = [[] for _ in range(BATCH_SIZE)]
			for batch in range(BATCH_SIZE):
				for prev_idx, next_idx in finesp_indices:
					n_next = self.n_image[batch, next_idx]
					if prev_idx == 0:
						n_prev = self.n_image[batch, prev_idx]
						prev_features = self.features[batch][prev_idx][:-1, :]
						rnn_features = tf.zeros((n_prev, 1, c_rnn), dtype=tf.float32)
					else:
						n_prev = self.n_image[batch, 0]
						sel = get_recur_sel(self.finesp_mats[batch][-1])
						rnn_sel = tf.stack([
							tf.range(n_prev, dtype=tf.int32),
							sel,
						], axis=1)
						prev_features = tf.gather(self.features[batch][prev_idx], sel, axis=0)
						rnn_features = tf.gather_nd(self.finesp_hiddens[batch][-1], rnn_sel)

					cur_mat, cur_hidden = get_mat_hidden(n_prev, n_next, rnn_features, prev_features, self.features[batch][next_idx], 'finesp', do_neg=False)
					self.finesp_mats[batch].append(cur_mat)
					self.finesp_hiddens[batch].append(cur_hidden)

			# longim loss
			self.longim_losses = []
			for batch in range(BATCH_SIZE):
				mat = self.extra_mats[batch][0]
				loss = compute_loss(mat, mat, batch)
				self.longim_losses.append(loss)
			self.longim_loss = tf.reduce_mean(self.longim_losses)

			# finespatial loss
			self.finesp_losses = []
			for batch in range(BATCH_SIZE):
				for i, finesp_mat in enumerate(self.finesp_mats[batch]):
					extra_mat = self.extra_mats_finesp[batch][i]
					extra_mat = tf.stop_gradient(extra_mat)
					loss = compute_loss(finesp_mat, extra_mat, batch, apply_mask=(i==SEQ_LEN-2))
					self.finesp_losses.append(loss)
			self.finesp_loss = tf.reduce_mean(self.finesp_losses)

			with tf.control_dependencies(tf.get_collection(tf.GraphKeys.UPDATE_OPS)):
				self.longim_optimizer = tf.train.AdamOptimizer(learning_rate=self.learning_rate).minimize(self.longim_loss)
				self.finesp_optimizer = tf.train.AdamOptimizer(learning_rate=self.learning_rate).minimize(self.finesp_loss)

		self.init_op = tf.global_variables_initializer()
		self.saver = tf.train.Saver(max_to_keep=None)
