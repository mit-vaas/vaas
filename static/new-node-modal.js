Vue.component('new-node-modal', {
	data: function() {
		return {
			newNodeFields: {
				name: '',
				dataType: '',
				type: null,
			},
			categories: [
				{
					ID: "models",
					Name: "Models",
					Types: [
						{
							ID: "yolov3",
							Name: "YOLOv3",
							Description: "Fast Object Detector",
							DataType: "detection",
							Parents: ["video"],
						},
						{
							ID: "simple-classifier",
							Name: "Simple Classifier",
							Description: "Simple Classification Model",
							DataType: "class",
							Parents: ["video"],
						},
					],
				},
				{
					ID: "filters",
					Name: "Filters",
					Types: [
						{
							ID: "filter-detection",
							Name: "Detection Filter",
							Description: "Filter Detections by Score or Category",
							DataType: "detection",
							Parents: ["detection"],
						},
						{
							ID: "filter-track",
							Name: "Track Filter",
							Description: "Filter Tracks based on Boxes",
							DataType: "track",
							Parents: ["track"],
						},
					],
				},
				{
					ID: "heuristics",
					Name: "Heuristics",
					Types: [
						{
							ID: "iou",
							Name: "IOU",
							Description: "Simple Overlap-based Multi-Object Tracker",
							DataType: "track",
							Parents: ["detection"],
						},
					],
				},
				{
					ID: "video",
					Name: "Video Manipulation",
					Types: [
						{
							ID: "crop",
							Name: "Crop",
							Description: "Crop video",
							DataType: "video",
							Parents: ["video"],
						},
						{
							ID: "rescale",
							Name: "Re-scale",
							Description: "Re-scale video",
							DataType: "video",
							Parents: ["video"],
						},
					],
				},
				{
					ID: "custom",
					Name: "Custom",
					Types: [
						{
							ID: "python",
							Name: "Python",
							Description: "Python function",
						},
					],
				},
				{
					ID: "misc",
					Name: "Miscellaneous",
					Types: [
						{
							ID: "downsample",
							Name: "Downsample",
							Description: "Downsample inputs to lower framerate",
						},
					],
				},
			],
		};
	},
	props: ['query_id'],
	mounted: function() {
		$(this.$refs.modal).modal('show');
	},
	methods: {
		createNode: function() {
			var params = {
				query_id: this.query_id,
				name: this.newNodeFields.name,
				type: this.newNodeFields.type.ID,
				dataType: this.newNodeFields.dataType,
			};
			$.post('/queries/nodes', params, () => {
				$(this.$refs.modal).modal('hide');
				this.$emit('closed');
			});
		},
		selectType: function(t) {
			this.newNodeFields.type = t;
			if(t.DataType) {
				this.newNodeFields.dataType = t.DataType;
			}
		},
	},
	template: `
<div class="modal" tabindex="-1" role="dialog" ref="modal">
	<div class="modal-dialog modal-xl" role="document">
		<div class="modal-content">
			<div class="modal-body">
				<form v-on:submit.prevent="createNode">
					<div class="form-group row">
						<label class="col-sm-2 col-form-label">Name</label>
						<div class="col-sm-10">
							<input v-model="newNodeFields.name" class="form-control" type="text" />
						</div>
					</div>
					<div class="form-group row">
						<label class="col-sm-2 col-form-label">Type</label>
						<div class="col-sm-10">
							<ul class="nav nav-tabs">
								<li v-for="category in categories" class="nav-item">
									<a
										class="nav-link"
										data-toggle="tab"
										:href="'#n-new-node-' + category.ID"
										role="tab"
										>
										{{ category.Name }}
									</a>
								</li>
							</ul>
							<div class="tab-content">
								<div v-for="category in categories" class="tab-pane" :id="'n-new-node-' + category.ID">
									<table class="table table-row-select">
										<thead>
											<tr>
												<th>Name</th>
												<th>Description</th>
											</tr>
										</thead>
										<tbody>
											<tr
												v-for="t in category.Types"
												:class="{selected: newNodeFields.type != null && newNodeFields.type.ID == t.ID}"
												v-on:click="selectType(t)"
												>
												<td>{{ t.Name }}</td>
												<td>{{ t.Description }}</td>
											</tr>
										</tbody>
									</table>
								</div>
							</div>
						</div>
					</div>
					<div class="form-group row">
						<label class="col-sm-2 col-form-label">Output</label>
						<div class="col-sm-10">
							<template v-if="newNodeFields.type != null && newNodeFields.type.DataType">
								<input type="text" readonly class="form-control-plaintext" :value="newNodeFields.type.DataType | capitalize">
							</template>
							<template v-else>
								<select v-model="newNodeFields.dataType" class="form-control">
									<option value=""></option>
									<option value="detection">Detection</option>
									<option value="track">Track</option>
									<option value="class">Class</option>
									<option value="video">Video</option>
									<option value="text">Text</option>
								</select>
							</template>
						</div>
					</div>
					<div class="form-group row">
						<div class="col-sm-10">
							<button type="submit" class="btn btn-primary">Create Node</button>
						</div>
					</div>
				</form>
			</div>
		</div>
	</div>
</div>
	`,
});
