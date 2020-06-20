Vue.component('new-node-modal', {
	data: function() {
		return {
			newNodeFields: {
				name: '',
				parents: '',
				type: '',
				ext: null,
			},
			categories: [
				{
					ID: "models",
					Name: "Models",
					Exts: [
						{
							ID: "yolov3",
							Name: "YOLOv3",
							Description: "Fast Object Detector",
							Type: "detection",
							Parents: ["detection"],
						},
					],
				},
				{
					ID: "filters",
					Name: "Filters",
					Exts: [
						{
							ID: "filter-detection",
							Name: "Detection Filter",
							Description: "Filter Detections by Score or Category",
							Type: "detection",
							Parents: ["detection"],
						},
						{
							ID: "filter-track",
							Name: "Track Filter",
							Description: "Filter Tracks based on Boxes",
							Type: "track",
							Parents: ["track"],
						},
					],
				},
				{
					ID: "heuristics",
					Name: "Heuristics",
					Exts: [
						{
							ID: "iou",
							Name: "IOU",
							Description: "Simple Overlap-based Multi-Object Tracker",
							Type: "track",
							Parents: ["detection"],
						},
					],
				},
				{
					ID: "video",
					Name: "Video Manipulation",
					Exts: [
						{
							ID: "crop",
							Name: "Crop",
							Description: "Crop video",
							Type: "video",
							Parents: ["video"],
						},
						{
							ID: "rescale",
							Name: "Re-scale",
							Description: "Re-scale video",
							Type: "video",
							Parents: ["video"],
						},
					],
				},
				{
					ID: "custom",
					Name: "Custom",
					Exts: [
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
					Exts: [
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
				parents: this.newNodeFields.parents,
				type: this.newNodeFields.type,
				ext: this.newNodeFields.ext.ID,
			};
			$.post('/queries/nodes', params, () => {
				$(this.$refs.modal).modal('hide');
				this.$emit('closed');
			});
		},
		selectExt: function(ext) {
			this.newNodeFields.ext = ext;
			if(ext.Type) {
				this.newNodeFields.type = ext.Type;
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
												v-for="e in category.Exts"
												:class="{selected: newNodeFields.ext != null && newNodeFields.ext.ID == e.ID}"
												v-on:click="selectExt(e)"
												>
												<td>{{ e.Name }}</td>
												<td>{{ e.Description }}</td>
											</tr>
										</tbody>
									</table>
								</div>
							</div>
						</div>
					</div>
					<div class="form-group row">
						<label class="col-sm-2 col-form-label">Parents</label>
						<div class="col-sm-10">
							<input v-model="newNodeFields.parents" class="form-control" type="text" />
						</div>
					</div>
					<div class="form-group row">
						<label class="col-sm-2 col-form-label">Output</label>
						<div class="col-sm-10">
							<template v-if="newNodeFields.ext != null && newNodeFields.ext.Type">
								<input type="text" readonly class="form-control-plaintext" :value="newNodeFields.ext.Type | capitalize">
							</template>
							<template v-else>
								<select v-model="newNodeFields.type" class="form-control">
									<option value=""></option>
									<option value="detection">Detection</option>
									<option value="track">Track</option>
									<option value="class">Class</option>
									<option value="video">Video</option>
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
