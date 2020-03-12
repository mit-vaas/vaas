function loadAnnotateVisualize(lsID) {
	var clipIndex, frameIndex, uuid;
	var width, height, videoURL;
	var state = 'preview';

	var update = function(data) {
		var div = $('#a-v-container');
		div.children().remove();
		div.css('width', data.Width + 'px');
		div.css('height', data.Height + 'px');
		var im = $('<img>')
			.attr('src', data.PreviewURL);
		div.append(im);

		uuid = data.UUID;
		width = data.Width;
		height = data.Height;
		videoURL = data.URL;
		state = 'preview';
		$('#a-d-index').text('');
	};

	$('#a-v-container').click(function(e) {
		if(state != 'preview') {
			return;
		}
		var div = $('#a-v-container');
		div.children().remove();
		var source = $('<source>')
			.attr('src', videoURL)
			.attr('type', 'video/mp4');
		var video = $('<video>')
			.attr('width', width)
			.attr('height', height)
			.attr('controls', true)
			.attr('autoplay', true)
			.append(source);
		div.append(video);
	});

	$('#a-v-prev').click(function() {
		if(index < 0) {
			$.get('/labelsets/labels?id='+lsID+'&index=0', updateImage, 'json');
		} else {
			var i = index-1;
			$.get('/labelsets/labels?id='+lsID+'&index='+i, updateImage, 'json');
		}
	});

	$('#a-v-next').click(function() {
		if(index < 0) {
			$.get('/labelsets/labels?id='+lsID+'&index=-1', updateImage, 'json');
		} else {
			var i = index+1;
			$.get('/labelsets/labels?id='+lsID+'&index='+i, updateImage, 'json');
		}
	});

	$.get('/labelsets/visualize?id='+lsID, update, 'json');
};
