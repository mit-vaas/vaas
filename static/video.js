function loadVideo() {
	$.get('/videos', function(data) {
		if(!data) {
			return;
		}
		data.forEach(function(el) {
			var row = $('<tr>');
			row.append($('<td>').text(el.Name));
			if(el.Percent == 100) {
				row.append($('<td>').text('Ready'));
			} else {
				row.append($('<td>').text(el.Percent + '%'));
			}
			$('#v-index-tbody').append(row);
		});
	}, 'json');

	$('#v-index-local').click(function() {
		$('.v-modal-input').val('');
		$('#v-local-modal').modal('show');
	});

	$('#v-index-youtube').click(function() {
		$('.v-modal-input').val('');
		$('#v-youtube-modal').modal('show');
	});

	$('#v-local-form').submit(function(e) {
		e.preventDefault();
		var params = {
			'name': $('#v-local-name').val(),
			'path': $('#v-local-path').val(),
		};
		$.post('/import/local', params, function() {
			$('#v-local-modal').modal('hide');
			myLoad('#video-panel', 'video-index.html', loadVideo);
		});
	});

	$('#v-youtube-form').submit(function(e) {
		e.preventDefault();
		var params = {
			'name': $('#v-youtube-name').val(),
			'url': $('#v-youtube-url').val(),
		};
		$.post('/import/youtube', params, function() {
			$('#v-youtube-modal').modal('hide');
			myLoad('#video-panel', 'video-index.html', loadVideo);
		});
	});
}
