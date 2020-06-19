Vue.component('nodes-tab', {
	data: function() {
		return {
			nodes: [],
			selectedNode: {},
			editor: '',

			newNodeFields: {},
			categories: [
				{
					ID: "models",
					Name: "Models",
					Exts: [
						{ID: "yolov3", Name: "YOLOv3", Description: "Fast Object Detector"},
					],
				},
				{
					ID: "filters",
					Name: "Filters",
					Exts: [
						{ID: "filter-detection", Name: "Detection Filter", Description: "Filter Detections by Score or Category"},
						{ID: "filter-track", Name: "Track Filter", Description: "Filter Tracks based on Boxes"},
					],
				},
				{
					ID: "heuristics",
					Name: "Heuristics",
					Exts: [
						{ID: "iou", Name: "IOU", Description: "Simple Overlap-based Multi-Object Tracker"},
					],
				},
				{
					ID: "video",
					Name: "Video Manipulation",
					Exts: [
						{ID: "crop", Name: "Crop", Description: "Crop video"},
					],
				},
				{
					ID: "custom",
					Name: "Custom",
					Exts: [
						{ID: "python", Name: "Python", Description: "Python function"},
					],
				},
			],
		};
	},
	props: ['tab'],
	created: function() {
		this.fetchNodes(true);
		setInterval(this.fetchNodes, 5000);
	},
	methods: {
		fetchNodes: function(force) {
			if(!force && this.tab != '#nodes-panel') {
				return;
			}
			$.get('/nodes', function(data) {
				this.nodes = data;
			}.bind(this));
		},
		showNewNodeModal: function() {
			this.newNodeFields = {
				name: '',
				parents: '',
				type: '',
				ext: '',
			};
			$('#n-new-node-modal').modal('show');
		},
		selectExt: function(ext) {
			this.newNodeFields.ext = ext;
		},
		editNode: function(node) {
			this.editor = '';
			Vue.nextTick(function() {
				this.selectedNode = node;
				if(this.selectedNode.Ext == 'python') {
					this.editor = 'node-edit-text';
				} else {
					this.editor = 'node-edit-' + this.selectedNode.Ext;
				}
			}.bind(this));
		},
		createNode: function() {
			$.post('/nodes', this.newNodeFields, function() {
				$('#n-new-node-modal').modal('hide');
				this.fetchNodes();
			}.bind(this));
		},
	},
	watch: {
		tab: function() {
			if(this.tab != '#nodes-panel') {
				return;
			}
			this.fetchNodes(true);
		},
	},
	template: `
<div id="n-div">
	<div id="n-organizer">
		<div id="n-node-list">
			<div v-for="node in nodes" class="m-1">
				<button
					v-on:click="editNode(node)"
					type="button"
					class="btn btn-sm n-btn-node"
					v-bind:class="{
						'btn-primary': node.ID != selectedNode.ID,
						'btn-success': node.ID == selectedNode.ID,
					}"
					>
					{{ node.Name }} ({{ node.ID }})
				</button>
			</div>
		</div>
		<div class="m-1">
			<button type="button" class="btn btn-primary" v-on:click="showNewNodeModal">New Node</button>
		</div>
	</div>
	<div id="n-edit-div">
		<component v-if="editor != ''" v-bind:is="editor" v-bind:initNode="selectedNode"></component>
	</div>
	<div class="modal" id="n-new-node-modal" tabindex="-1" role="dialog">
		<div class="modal-dialog modal-lg" role="document">
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
							<label class="col-sm-2 col-form-label">Parents</label>
							<div class="col-sm-10">
								<input v-model="newNodeFields.parents" class="form-control" type="text" />
							</div>
						</div>
						<div class="form-group row">
							<label class="col-sm-2 col-form-label">Output</label>
							<div class="col-sm-10">
								<select v-model="newNodeFields.type" class="form-control">
									<option value=""></option>
									<option value="detection">Detection</option>
									<option value="track">Track</option>
									<option value="class">Class</option>
									<option value="video">Video</option>
								</select>
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
													:class="{selected: newNodeFields.ext != null && newNodeFields.ext == e.ID}"
													v-on:click="selectExt(e.ID)"
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
							<div class="col-sm-10">
								<button type="submit" class="btn btn-primary">Create Node</button>
							</div>
						</div>
					</form>
				</div>
			</div>
		</div>
	</div>
</div>
	`,
});
