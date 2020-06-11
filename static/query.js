function loadQuery() {
	var loadNode = function(nodeID) {
		$.get('/node', {id: nodeID}, function(data) {
			$('#q-node-code').val(data.Code);
			$('#q-node-code').data('node_id', data.ID);
		}, 'json');
	};

	$('#q-node-save-btn').click(function() {
		var nodeID = $('#q-node-code').data('node_id');
		var code = $('#q-node-code').val();
		$.post('/node?id='+nodeID, {
			code: code,
		});
	});

	$.get('/nodes', function(data) {
		if(!data) {
			return;
		}
		var div = $('#q-node-list');
		div.children().remove();
		data.forEach(function(el) {
			var btn = $('<button>')
				.attr('type', 'button')
				.addClass('btn btn-primary btn-sm q-btn-node')
				.text(el.Name + ' (' + el.ID + ')');
			div.append(btn);

			btn.click(function() {
				$('.q-btn-node').removeClass('btn-success');
				btn.addClass('btn-success');
				loadNode(el.ID);
			});
		});
	}, 'json');

	$.get('/videos', function(data) {
		if(!data) {
			return;
		}
		data.forEach(function(el) {
			var option = $('<option>')
				.attr('value', el.ID)
				.text(el.Name);
			$('#q-exec-video').append(option);
		});
	}, 'json');

	// auto indentation
	$('#q-node-code').keydown(function(e) {
		if(e.keyCode == 9) {
			// tab
			e.preventDefault();
			var start = this.selectionStart;
			var prefix = this.value.substring(0, start);
			var suffix = this.value.substring(this.selectionEnd);
			this.value = prefix + '\t' + suffix;
			this.selectionStart = start+1;
			this.selectionEnd = start+1;
		} else if(e.keyCode == 13) {
			// enter
			e.preventDefault();
			var start = this.selectionStart;
			var prefix = this.value.substring(0, start);
			var suffix = this.value.substring(this.selectionEnd);
			var prevLine = this.value.lastIndexOf('\n', start);

			var spacing = '';
			if(prevLine != -1) {
				for(var i = prevLine+1; i < start; i++) {
					var char = this.value[i];
					if(char != '\t' && char != ' ') {
						break;
					}
					spacing += char;
				}
			}

			this.value = prefix + '\n' + spacing + suffix;
			this.selectionStart = start+1+spacing.length;
			this.selectionEnd = this.selectionStart;
		}
	});

	var testOne = function(div) {
		return function(data) {
			var im = $('<img>')
				.attr('src', data.PreviewURL)
				.addClass('q-result-img');
			div.append(im);

			uuid = data.UUID;
			im.click(function(e) {
				div.children().remove();
				var source = $('<source>')
					.attr('src', data.URL)
					.attr('type', 'video/mp4');
				var video = $('<video>')
					.attr('width', data.Width)
					//.attr('height', data.Height)
					.addClass('q-result-img')
					.attr('controls', true)
					.attr('autoplay', true)
					.append(source);
				div.append(video);
			});
		}
	};

	var addMore = function() {
		var req = {
			video_id: $('#q-exec-video').val(),
			query_id: $('#q-exec-query').val()
		};
		var row = $('<div>')
			.addClass('q-results-row');
		for(var i = 0; i < 4; i++) {
			var col = $('<div>')
				.addClass('q-results-col');
			row = row.append(col);
			if(false) {
				$.post('/exec/test2', req, testOne(col), 'json');
			} else {
				$.post('/exec/test', req, testOne(col), 'json');
			}
		}
		$('.q-results-more-btn').before(row);
	};

	$('#q-exec-test-btn').click(function() {
		var div = $('#q-results');
		div.children().remove();
		var btn = $('<button>')
			.addClass('btn btn-primary q-results-more-btn')
			.text('More');
		btn.click(function() {
			addMore();
		});
		div.append(btn);
		addMore();
	});

	$('#q-new-node-btn').click(function() {
		$('.q-modal-input').val('');
		$('#q-new-node-unit').val('750');
		$('#q-new-node-modal').modal('show');
	});

	$('#q-new-node-form').submit(function(e) {
		e.preventDefault();
		var params = {
			'name': $('#q-new-node-name').val(),
			'parents': $('#q-new-node-parents').val(),
			'unit': $('#q-new-node-unit').val(),
			'type': $('#q-new-node-type').val(),
			'ext': $('#q-new-node-ext').val(),
		};
		$.post('/nodes', params, function() {
			$('#q-new-node-modal').modal('hide');
			myLoad('#query-panel', 'query.html', loadQuery);
		});
	});
}
