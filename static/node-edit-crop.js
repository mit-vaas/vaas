Vue.component('node-edit-crop', {
	data: function() {
		return {
			left: '',
			top: '',
			right: '',
			bottom: '',
			dataSeries: [],
			selectedSeries: '',
			drawSeries: null,
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.left = s.left;
			this.top = s.top;
			this.right = s.right;
			this.bottom = s.bottom;
		} catch(e) {}
		myCall('GET', '/datasets', null, (data) => {
			this.dataSeries = data;
		});
	},
	methods: {
		showSeries: function() {
			if(!this.selectedSeries) {
				return;
			}
			this.drawSeries = this.selectedSeries;
		},
		onDraw: function(box) {
			this.left = box.left;
			this.top = box.top;
			this.right = box.right;
			this.bottom = box.bottom;
		},
		save: function() {
			var code = JSON.stringify({
				left: parseInt(this.left),
				top: parseInt(this.top),
				right: parseInt(this.right),
				bottom: parseInt(this.bottom),
			});
			myCall('POST', '/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
	},
	template: `
<div class="m-2">
	<p>The coordinates below specify the crop. To obtain the coordinates, select a video dataset, and then draw a box corresponding to the desired crop window.</p>
	<table>
		<tbody>
			<tr>
				<td>Left</td>
				<td>
					<input v-model="left" type="text" class="form-control short-input">
			 	</td>
			</tr>
			<tr>
				<td>Top</td>
				<td>
					<input v-model="top" type="text" class="form-control short-input">
			 	</td>
			</tr>
			<tr>
				<td>Right</td>
				<td>
					<input v-model="right" type="text" class="form-control short-input">
			 	</td>
			</tr>
			<tr>
				<td>Bottom</td>
				<td>
					<input v-model="bottom" type="text" class="form-control short-input">
			 	</td>
			</tr>
		</tbody>
	</table>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
	<form v-on:submit.prevent="showSeries" class="form-inline my-2">
		<label>Draw Window over Video</label>
		<select v-model="selectedSeries" class="form-control mx-2">
			<option v-for="ds in dataSeries" :value="ds.ID">{{ ds.Name }}</option>
		</select>
		<button type="submit" class="btn btn-primary mx-2">Select</button>
	</form>
	<div v-if="drawSeries != null">
		<util-video-draw-shape v-bind:series_id="drawSeries" fixedOptions="box" v-on:draw="onDraw($event)"></util-video-draw-shape>
	</div>
</div>
	`,
});
