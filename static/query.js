function loadQuery() {
	var loadOp = function(opID) {
		$.get('/op', {id: opID}, function(data) {
			$('#q-op-code').val(data.Code);
			$('#q-op-code').data('opid', data.ID);
		}, 'json');
	};

	$('#q-op-save-btn').click(function() {
		var opID = $('#q-op-code').data('opid');
		var code = $('#q-op-code').val();
		$.post('/op?id='+opID, {
			code: code,
		});
	});

	$.get('/ops', function(data) {
		if(!data) {
			return;
		}
		var div = $('#q-op-list');
		div.children().remove();
		data.forEach(function(el) {
			var btn = $('<button>')
				.attr('type', 'button')
				.addClass('btn btn-primary btn-sm q-btn-op')
				.text(el.Name);
			div.append(btn);

			btn.click(function() {
				$('.q-btn-op').removeClass('btn-success');
				btn.addClass('btn-success');
				loadOp(el.ID);
			});
		});
	}, 'json');

	// auto indentation
	$('#q-op-code').keydown(function(e) {
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
			video_id: 1,
			query: $('#q-op-query').val()
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

	$('#q-op-test-btn').click(function() {
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
}
