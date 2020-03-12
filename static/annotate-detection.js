function loadDetectionLabeler(lsID) {
	var layer1 = null;
	var layer2 = null;
	var context1 = null;
	var context2 = null;

	var index = -1;
	var uuid = 0;
	var labels = [];
	var working = [];
	var mode = $('#a-d-mode').val();
	var state = 'idle';

	var cancelWorking = function() {
		context2.clearRect(0, 0, layer2.width, layer2.height);
		state = 'idle';
		working = [];
	};

	var render = function() {
		context1.clearRect(0, 0, layer1.width, layer1.height);
		if(mode == 'line') {
			labels.forEach(function(el) {
				context1.beginPath();
				context1.moveTo(el[0][0], el[0][1]);
				context1.lineTo(el[1][0], el[1][1]);
				context1.lineWidth = 3;
				context1.strokeStyle = '#ff0000';
				context1.stroke();
				context1.closePath();
			});
		}
	};

	var updateImage = function(data) {
		var div = $('#a-d-container');
		div.children().remove();
		div.css('width', data.Width + 'px');
		div.css('height', data.Height + 'px');
		var im = $('<img>')
			.attr('src', data.URL);
		var canvas1 = $('<canvas>')
			.attr('width', data.Width)
			.attr('height', data.Height)
			.addClass('layer1');
		var canvas2 = $('<canvas>')
			.attr('width', data.Width)
			.attr('height', data.Height)
			.addClass('layer2');
		div.append(im);
		div.append(canvas1);
		div.append(canvas2);

		layer1 = $('.layer1')[0];
		layer2 = $('.layer2')[0];
		context1 = layer1.getContext('2d');
		context2 = layer2.getContext('2d');

		index = data.Index;
		uuid = data.UUID;
		if(data.Labels) {
			labels = data.Labels;
		} else {
			labels = [];
		}
		working = [];
		state = 'idle';

		if(index < 0) {
			$('#a-d-index').text('[New]');
		} else {
			$('#a-d-index').text(index);
		}

		render();
	};

	$('#a-d-mode').change(function() {
		mode = $('#a-d-mode').val();
		render();
	});

	$('#a-d-container').click(function(e) {
		var rect = e.target.getBoundingClientRect();
		var x = e.clientX - rect.left;
		var y = e.clientY - rect.top;

		context2.clearRect(0, 0, layer2.width, layer2.height);
		if(e.which == 3) {
			if(state == 'idle') {
				return;
			}
			e.preventDefault();
			cancelWorking();
		} else if(state == 'idle') {
			if(mode == 'point') {
				// ...
			} else if(mode == 'line') {
				state = 'line';
				working.push([x, y]);
			}
		} else if(state == 'line') {
			var line = [[working[0][0], working[0][1]], [x, y]];
			labels.push(line);
			cancelWorking();
			render();
		}
	});

	keyHandler = function(e) {
		console.log(e);
		if(e.key == 'x') {
			cancelWorking();
		}
	};

	$('#a-d-container').mousemove(function(e) {
		var rect = e.target.getBoundingClientRect();
		var x = e.clientX - rect.left;
		var y = e.clientY - rect.top;

		context2.clearRect(0, 0, layer2.width, layer2.height);
		if(state == 'line') {
			context2.beginPath();
			context2.moveTo(working[0][0], working[0][1]);
			context2.lineTo(x, y);
			context2.lineWidth = 3;
			context2.strokeStyle = '#ff0000';
			context2.stroke();
			context2.closePath();
		}
	});

	$('#a-d-prev').click(function() {
		if(index < 0) {
			$.get('/labelsets/labels?id='+lsID+'&index=0', updateImage, 'json');
		} else {
			var i = index-1;
			$.get('/labelsets/labels?id='+lsID+'&index='+i, updateImage, 'json');
		}
	});

	$('#a-d-next').click(function() {
		if(index < 0) {
			$.get('/labelsets/labels?id='+lsID+'&index=-1', updateImage, 'json');
		} else {
			var i = index+1;
			$.get('/labelsets/labels?id='+lsID+'&index='+i, updateImage, 'json');
		}
	});

	$('#a-d-done').click(function() {
		var req = {
			id: lsID,
			index: index,
			uuid: uuid,
			labels: labels,
		};
		$.ajax({
			type: "POST",
			url: '/labelsets/detection-label',
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
	});

	$('#a-d-clear').click(function() {
		labels = [];
		render();
	});

	$.get('/labelsets/labels?id='+lsID+'&index=-1', updateImage, 'json');
};
