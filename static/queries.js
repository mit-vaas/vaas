Vue.component('queries-tab', {
	data: function() {
		return {
			queries: [],
			nodes: [],
			newQueryName: '',
			selectedQueryID: '',
			selectedQuery: null,
			selectedNode: null,
			showNewNodeModal: false,
			nodeRects: {},
			editor: '',
			prevStage: null,
			resizeObserver: null,

			addParentFields: {},
		};
	},
	props: ['tab'],
	// don't want to render until after mounted
	mounted: function() {
		this.fetchQueries(true);
		setInterval(this.fetchQueries, 5000);
		Vue.nextTick(() => {
			app.$on('showQuery', (query_id) => {
				this.selectedQueryID = query_id;
				this.update();
				app.changeTab('#queries-panel');
			});
		});
	},
	methods: {
		fetchQueries: function(force) {
			if(!force && this.tab != '#queries-panel') {
				return;
			}
			$.get('/queries', (queries) => {
				this.queries = queries;
				if(this.selectedQueryID == '' && this.queries.length > 0) {
					this.selectedQueryID = this.queries[0].ID;
					this.update();
				}
			});
		},
		createQuery: function() {
			$.post('/queries', {name: this.newQueryName}, (query) => {
				this.newQueryName = '';
				this.fetchQueries();
				this.queries.push(query);
				this.selectedQueryID = query.ID;
				this.update();
			});
		},
		update: function() {
			if(this.selectedQueryID == '') {
				return;
			}
			$.get('/queries/query?query_id='+this.selectedQueryID, (query) => {
				this.selectedQuery = query;
				if(this.selectedNode && query.Nodes[this.selectedNode.ID]) {
					this.selectNode(query.Nodes[this.selectedNode.ID]);
				} else {
					this.selectedNode = null;
				}
				this.render();
			})
		},
		render: function() {
			var query = this.selectedQuery;
			var dims = [1000, 500];
			var scale = (this.$refs.view.offsetWidth-10) / dims[0];

			if(this.prevStage) {
				this.prevStage.destroy();
			}
			if(this.resizeObserver) {
				this.resizeObserver.disconnect();
			}

			var stage = new Konva.Stage({
				container: this.$refs.layer,
				width: parseInt(dims[0]*scale),
				height: parseInt(dims[1]*scale),
			});
			this.prevStage = stage;

			var layer = new Konva.Layer();
			var rescaleLayer = () => {
				if(!this.$refs.view) {
					// probably editing node
					return;
				}
				var scale = (this.$refs.view.offsetWidth-10) / dims[0];
				stage.width(parseInt(dims[0]*scale));
				stage.height(parseInt(dims[1]*scale));
				layer.scaleX(scale);
				layer.scaleY(scale);
				layer.draw();
			};
			rescaleLayer();
			this.resizeObserver = new ResizeObserver(rescaleLayer);
			this.resizeObserver.observe(this.$refs.view);
			stage.add(layer);
			layer.add(new Konva.Rect({
				x: 0,
				y: 0,
				width: 1000,
				height: 700,
				fill: 'lightgrey',
			}));

			if(!query.Nodes) {
				query.Nodes = {};
			}
			if(!query.RenderMeta) {
				query.RenderMeta = {};
			}

			var groups = {};
			var arrows = {};

			var save = () => {
				var meta = {};
				for(var gid in groups) {
					meta[gid] = [parseInt(groups[gid].x()), parseInt(groups[gid].y())];
				}
				var params = {
					ID: this.selectedQueryID,
					Meta: meta,
				};
				$.ajax({
					type: "POST",
					url: '/queries/render-meta',
					data: JSON.stringify(params),
					processData: false,
				});
			};

			var addGroup = (id, text, meta) => {
				var text = new Konva.Text({
					x: 0,
					y: 0,
					text: text,
					padding: 5,
				});
				text.offsetX(text.width() / 2);
				text.offsetY(text.height() / 2);
				var rect = new Konva.Rect({
					x: 0,
					y: 0,
					offsetX: text.offsetX(),
					offsetY: text.offsetY(),
					width: text.width(),
					height: text.height(),
					stroke: 'black',
					strokeWidth: 1,
					name: 'myrect',
				});
				if(this.selectedNode != null && 'n' + this.selectedNode.ID == id) {
					rect.fill('salmon');
				}
				var group = new Konva.Group({
					draggable: true,
					x: meta[0],
					y: meta[1],
				});
				group.mywidth = text.width();
				group.myheight = text.height();
				group.myrect = rect;
				group.on('dragend', save);
				group.add(rect);
				group.add(text);
				layer.add(group);
				groups[id] = group;
				return group;
			};

			var resetColors = () => {
				for(let gid in groups) {
					let rect = groups[gid].myrect;
					if(gid[0] == 's') {
						rect.fill('lightgreen');
					} else {
						rect.fill('lightblue');
					}
				}
				query.Outputs.forEach((section) => {
					section.forEach((parent) => {
						groups[parent.Spec].myrect.fill('mediumpurple');
					});
				});
				if(this.selectedNode != null) {
					groups['n'+this.selectedNode.ID].myrect.fill('salmon');
				}
				layer.draw();
			};

			// (1) render the vector inputs
			var numSources = 1;
			for(var nodeID in query.Nodes) {
				query.Nodes[nodeID].Parents.forEach((parent) => {
					if(parent.Type != 's') {
						return;
					}
					if(parent.SeriesIdx+1 > numSources) {
						numSources = parent.SeriesIdx+1;
					}
				});
			}
			for(let i = 0; i < numSources; i++) {
				let meta = query.RenderMeta['s' + i];
				if(!meta) {
					meta = [50+i*200, 50];
				}
				addGroup('s'+i, `Input ${i}`, meta);
			}

			// (2) render the nodes
			var numDefault = 0;
			for(let nodeID in query.Nodes) {
				let node = query.Nodes[nodeID];
				let meta = query.RenderMeta['n' + nodeID];
				if(!meta) {
					meta = [500, 150+25*numDefault];
					numDefault++;
				}
				let group = addGroup('n'+nodeID, `${node.Name} (${node.Type})`, meta);
				let rect = group.myrect;

				group.on('mouseenter', () => {
					stage.container().style.cursor = 'pointer';
				})
				group.on('mouseleave', () => {
					stage.container().style.cursor = 'default';
				})
				group.on('click', (e) => {
					e.cancelBubble = true;
					this.selectNode(node);
					resetColors();
				});
			}

			resetColors();

			stage.on('click', (e) => {
				this.selectNode(null);
				resetColors();
			});

			// (3) render the arrows
			var getClosestPoint = (group1, group2, isdst) => {
				var cx = group1.x();
				var cy = group1.y();
				var width = group1.mywidth;
				var height = group1.myheight;
				var padding = 0;
				if(isdst) {
					// add padding so arrow doesn't go into the rectangle
					padding = 3;
				}
				var p1 = [
					[cx, cy-height/2-padding],
					[cx, cy+height/2+padding],
					[cx-width/2-padding, cy],
					[cx+width/2+padding, cy],
				];
				var p2 = [group2.x(), group2.y()];
				var best = null;
				var bestDistance = 0;
				p1.forEach((p) => {
					var dx = p[0]-p2[0];
					var dy = p[1]-p2[1];
					var d = dx*dx+dy*dy;
					if(best == null || d < bestDistance) {
						best = p;
						bestDistance = d;
					}
				});
				return best;
			};
			for(var nodeID in query.Nodes) {
				var node = query.Nodes[nodeID];
				if(!node.Parents) {
					continue;
				}
				var dst = 'n'+nodeID;
				node.Parents.forEach((parent) => {
					if(parent.Type == 'n') {
						var src = 'n'+parent.NodeID;
					} else if(parent.Type == 's') {
						var src = 's'+parent.SeriesIdx;
					}
					var p1 = getClosestPoint(groups[src], groups[dst], false);
					var p2 = getClosestPoint(groups[dst], groups[src], true);
					var arrow = new Konva.Arrow({
						points: [p1[0], p1[1], p2[0], p2[1]],
						pointerLength: 10,
						pointerWidth: 10,
						fill: 'black',
						stroke: 'black',
						strokeWidth: 2,
					});
					layer.add(arrow);
					if(!(src in arrows)) {
						arrows[src] = [];
					}
					if(!(dst in arrows)) {
						arrows[dst] = [];
					}
					arrows[src].push(['src', arrow, dst]);
					arrows[dst].push(['dst', arrow, src]);
				});
			}

			// (4) add listeners to move the arrows when groups are dragged
			for(let gid in arrows) {
				let l = arrows[gid];
				groups[gid].on('dragmove', function() {
					l.forEach(function(el) {
						let mode = el[0];
						let arrow = el[1];
						let other = el[2];
						let p1, p2;
						if(mode == 'src') {
							p1 = getClosestPoint(groups[gid], groups[other], false);
							p2 = getClosestPoint(groups[other], groups[gid], true);
						} else {
							p1 = getClosestPoint(groups[other], groups[gid], false);
							p2 = getClosestPoint(groups[gid], groups[other], true);
						}
						let points = [p1[0], p1[1], p2[0], p2[1]];
						arrow.points(points);
						layer.draw();
					});
				});
			}

			layer.draw();
		},
		onNewNodeModalClosed: function() {
			this.showNewNodeModal = false;
			this.update();
		},
		selectNode: function(node) {
			this.selectedNode = node;
			if(node) {
				node.parentSet = {};
				node.Parents.forEach((parent) => {
					node.parentSet[parent.Spec] = parent;
				});
			}
			this.addParentFields = {spec: ''};
		},
		editNode: function() {
			if(this.selectedNode.Type == 'python') {
				this.editor = 'node-edit-text';
			} else {
				this.editor = 'node-edit-' + this.selectedNode.Type;
			}
		},
		removeNode: function() {
			$.post('/queries/node/remove', {id: this.selectedNode.ID}, () => {
				this.update();
			});
		},
		backFromEditing: function() {
			this.editor = '';
			this.update();
		},
		removeParent: function(spec) {
			let parents = this.selectedNode.Parents.filter(parent => parent.Spec != spec);
			let parts = parents.map((parent) => parent.Spec);
			let parentsStr = parts.join(',');
			$.post('/queries/node?id='+this.selectedNode.ID, {parents: parentsStr}, () => {
				this.update();
			});
		},
		addParent: function(spec) {
			let parts = this.selectedNode.Parents.map((parent) => parent.Spec);
			parts.push(spec);
			let parentsStr = parts.join(',');
			$.post('/queries/node?id='+this.selectedNode.ID, {parents: parentsStr}, () => {
				this.update();
			});
		},
		saveOutputs: function(outputs) {
			let parts = [];
			outputs.forEach((section) => {
				var l = [];
				section.forEach((parent) => {
					l.push(parent.Spec);
				});
				parts.push(l.join(','));
			});
			let outputsStr = parts.join(';');
			$.post('/queries/query?query_id='+this.selectedQuery.ID, {outputs: outputsStr}, () => {
				this.update();
			});
		},
		addOutputRow: function() {
			let outputs = this.selectedQuery.Outputs;
			outputs.push([]);
			this.saveOutputs(outputs);
		},
		removeOutputRow: function(i) {
			let outputs = this.selectedQuery.Outputs;
			outputs.splice(i, 1);
			this.saveOutputs(outputs);
		},
		addOutput: function(i, spec) {
			let outputs = this.selectedQuery.Outputs;
			outputs[i].push({Spec: spec});
			this.saveOutputs(outputs);
		},
		removeOutput: function(i, spec) {
			let outputs = this.selectedQuery.Outputs;
			outputs[i] = outputs[i].filter((parent) => parent.Spec != spec);
			this.saveOutputs(outputs);
		},
	},
	watch: {
		tab: function() {
			if(this.tab != '#queries-panel') {
				return;
			}
			this.fetchQueries(true);
			this.update();
		},
	},
	template: `
<div style="height: 100%">
	<div v-if="editor == ''" id="q-div">
		<div id="q-view" ref="view">
			<form v-on:submit.prevent="createQuery" class="form-inline my-2">
				<label class="ml-1">Query:</label>
				<select v-model="selectedQueryID" @change="update" class="form-control ml-1">
					<option v-for="query in queries" :value="query.ID">{{ query.Name }}</option>
				</select>
				<button type="button" class="btn btn-danger ml-1">Remove</button>
				<input v-model="newQueryName" type="form-control" placeholder="Name" class="ml-4" />
				<button type="submit" class="btn btn-primary ml-1">Add Query</button>
			</form>
			<div ref="layer"></div>
			<div v-if="selectedQuery != null" class="small-container">
				<template v-for="(outputs, i) in selectedQuery.Outputs">
					Output {{ i }} <button type="button" class="btn btn-danger" v-on:click="removeOutputRow(i)">Remove</button>
					<queries-parents-table
						:query="selectedQuery"
						:parents="outputs"
						label="Outputs"
						v-on:add="addOutput(i, $event)"
						v-on:remove="removeOutput(i, $event)"
						>
					</queries-parents-table>
				</template>
				<button type="button" class="btn btn-primary" v-on:click="addOutputRow">Add Output</button>
			</div>
		</div>
		<div>
			<div class="my-2">
				<button type="button" class="btn btn-primary" v-on:click="showNewNodeModal = true">New Node</button>
				<button type="button" class="btn btn-primary" :disabled="selectedNode == null" v-on:click="editNode">Edit Node</button>
			</div>
			<hr />
			<div v-if="selectedNode != null" class="my-2">
				<div>Node {{ selectedNode.Name }}</div>
				<div><button type="button" class="btn btn-danger" v-on:click="removeNode">Remove Node</button></div>
				<div>
					<queries-parents-table
						:query="selectedQuery"
						:parents="selectedNode.Parents"
						label="Parents"
						v-on:add="addParent($event)"
						v-on:remove="removeParent($event)"
						>
					</queries-parents-table>
				</div>
			</div>
		</div>
		<new-node-modal v-if="showNewNodeModal && selectedQueryID != ''" :query_id="selectedQueryID" v-on:closed="onNewNodeModalClosed"></new-node-modal>
	</div>
	<div v-else id="q-node-edit-container">
		<div>
			<button type="button" class="btn btn-primary" v-on:click="backFromEditing">Back</button>
		</div>
		<div id="q-node-edit-div">
			<component v-bind:is="editor" v-bind:initNode="selectedNode"></component>
		</div>
	</div>
</div>
	`,
});
