function loadAnnotate() {
	var loadAnnotateAdd = function() {
		$.get('/videos', function(data) {
			if(!data) {
				return;
			}
			data.forEach(function(el) {
				var option = $('<option>')
					.attr('value', el.ID)
					.text(el.Name);
				$('#a-add-src').append(option);
			});
		}, 'json');

		$('#a-add-form').submit(function(e) {
			e.preventDefault();
			var req = {
				name: $('#a-add-name').val(),
				type: $('#a-add-type').val(),
				src: $('#a-add-src').val(),
			};
			$.post('/labelsets', req, function(ls) {
				myLoad('#annotate-panel', 'annotate-detection.html', function() {
					loadDetectionLabeler(ls.ID);
				});
			}, 'json');
		});
	};

	$.get('/labelsets', function(data) {
		if(!data) {
			return;
		}
		data.forEach(function(el) {
			var row = $('<tr>');
			row.append($('<td>').text(el.Name));
			row.append($('<td>').text(el.SrcVideo.Name));
			row.append($('<td>').text(el.Type));

			var annotateBtn = $('<btn>')
				.attr('type', 'button')
				.addClass('btn btn-primary btn-sm')
				.text('Annotate');
			var visualizeBtn = $('<btn>')
				.attr('type', 'button')
				.addClass('btn btn-primary btn-sm')
				.text('Visualize');
			var item = $('<td>')
				.append(annotateBtn)
				.append(visualizeBtn);
			row.append(item);
			$('#a-index-tbody').append(row);

			annotateBtn.click(function(e) {
				myLoad('#annotate-panel', 'annotate-detection.html', function() {
					loadDetectionLabeler(el.ID);
				});
			});
			visualizeBtn.click(function(e) {
				myLoad('#annotate-panel', 'annotate-visualize.html', function() {
					loadAnnotateVisualize(el.ID);
				});
			});
		});
	}, 'json');

	$('#a-index-add').click(function() {
		myLoad('#annotate-panel', 'annotate-add.html', loadAnnotateAdd);
	});
}
