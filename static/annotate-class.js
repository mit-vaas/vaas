function loadClassLabeler(lsID) {
	var index = -1;
	var uuid = 0;

	var updateImage = function(data) {
		var div = $('#a-c-container');
		div.children().remove();
		div.css('width', data.Width + 'px');
		div.css('height', data.Height + 'px');
		var im = $('<img>')
			.attr('src', data.URL);
		div.append(im);

		index = data.Index;
		uuid = data.UUID;

		if(index < 0) {
			$('#a-c-index').text('[New]');
		} else {
			if(data.Labels) {
				if(data.Labels[0] == 1) {
					$('#a-c-index').text(index + ' (Positive)');
				} else {
					$('#a-c-index').text(index + ' (Negative)');
				}
			} else {
				$('#a-c-index').text(index);
			}
		}
	};

	$('#a-c-prev').click(function() {
		if(index < 0) {
			$.get('/labelsets/labels?id='+lsID+'&index=0', updateImage, 'json');
		} else {
			var i = index-1;
			$.get('/labelsets/labels?id='+lsID+'&index='+i, updateImage, 'json');
		}
	});

	$('#a-c-next').click(function() {
		if(index < 0) {
			$.get('/labelsets/labels?id='+lsID+'&index=-1', updateImage, 'json');
		} else {
			var i = index+1;
			$.get('/labelsets/labels?id='+lsID+'&index='+i, updateImage, 'json');
		}
	});

	var done = function(label) {
		var req = {
			id: lsID,
			index: index,
			uuid: uuid,
			label: [label],
		};
		$.ajax({
			type: "POST",
			url: '/labelsets/class-label',
			data: JSON.stringify(req),
			processData: false,
			success: function() {
				if(index < 0) {
					$.get('/labelsets/labels?id='+lsID+'&index=-1', updateImage, 'json');
				} else {
					var i = index+1;
					$.get('/labelsets/labels?id='+lsID+'&index='+i, updateImage, 'json');
				}
			},
		});
	};

	$('#a-c-positive').click(function() {
		done(1);
	});

	$('#a-c-negative').click(function() {
		done(0);
	});

	$.get('/labelsets/labels?id='+lsID+'&index=-1', updateImage, 'json');
};
