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
			var type = $('#a-add-type').val();
			var req = {
				name: $('#a-add-name').val(),
				type: type,
				src: $('#a-add-src').val(),
			};
			$.post('/labelsets', req, function(ls) {
				if(type == 'detection') {
					myLoad('#annotate-panel', 'annotate-detection.html', function() {
						loadDetectionLabeler(ls.ID);
					});
				} else if(type == 'class') {
					myLoad('#annotate-panel', 'annotate-class.html', function() {
						loadClassLabeler(ls.ID);
					});
				}
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
				.addClass('btn btn-primary btn-sm mx-1')
				.text('Annotate');
			var visualizeBtn = $('<btn>')
				.attr('type', 'button')
				.addClass('btn btn-primary btn-sm mx-1')
				.text('Visualize');
			var item = $('<td>')
				.append(annotateBtn)
				.append(visualizeBtn);
			row.append(item);
			$('#a-index-tbody').append(row);

			annotateBtn.click(function(e) {
				if(el.Type == "detection") {
					myLoad('#annotate-panel', 'annotate-detection.html', function() {
						loadDetectionLabeler(el.ID);
					});
				} else if(el.Type == "class") {
					myLoad('#annotate-panel', 'annotate-class.html', function() {
						loadClassLabeler(el.ID);
					});
				}
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
