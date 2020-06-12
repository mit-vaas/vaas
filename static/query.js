function loadQuery() {
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
}
