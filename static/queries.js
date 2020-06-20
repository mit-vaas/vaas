Vue.component('queries-tab', {
	data: function() {
		return {
			queries: [],
			nodes: [],
			newQueryName: '',
			selectedQuery: '',
			selectedNode: null,
			showNewNodeModal: false,
			nodeRects: {},
			editor: '',
		};
	},
	props: ['tab'],
	// don't want to render until after mounted
	mounted: function() {
		this.fetchQueries(true);
		setInterval(this.fetchQueries, 5000);
	},
	methods: {
		fetchQueries: function(force) {
			if(!force && this.tab != '#queries-panel') {
				return;
			}
			$.get('/queries', (queries) => {
				this.queries = queries;
				if(this.selectedQuery == '') {
					this.selectedQuery = this.queries[0].ID;
					this.update();
				}
			});
		},
		createQuery: function() {
			$.post('/queries', {name: this.newQueryName}, (query) => {
				this.newQueryName = '';
				this.fetchQueries();
				this.queries.push(query);
				this.selectedQuery = query.ID;
				this.update();
			});
		},
		update: function() {
			if(this.selectedQuery == '') {
				return;
			}
			$.get('/queries/query?query_id='+this.selectedQuery, (query) => {
				this.render(query);
			})
		},
		render: function(query) {
			var dims = [1000, 500];
			var stage = new Konva.Stage({
				container: this.$refs.layer,
				width: dims[0],
				height: dims[1],
			});
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
			new ResizeObserver(rescaleLayer).observe(this.$refs.view);
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
					fill: 'lightblue',
					stroke: 'black',
					strokeWidth: 1,
					name: 'myrect',
				});
				var group = new Konva.Group({
					draggable: true,
					x: meta[0],
					y: meta[1],
				});
				group.mywidth = text.width();
				group.myheight = text.height();
				group.add(rect);
				group.add(text);
				layer.add(group);
				groups[id] = group;
				return [rect, group];
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
				let shapes = addGroup('n'+nodeID, `${node.Name} (${node.Ext})`, meta);
				let rect = shapes[0];
				let group = shapes[1];

				group.on('mouseenter', () => {
					stage.container().style.cursor = 'pointer';
				})
				group.on('mouseleave', () => {
					stage.container().style.cursor = 'default';
				})
				group.on('click', (e) => {
					e.cancelBubble = true;
					this.selectedNode = node;
					layer.find('.myrect').fill('lightblue');
					rect.fill('salmon');
					layer.draw();
				});
			}

			stage.on('click', (e) => {
				this.selectedNode = null;
				layer.find('.myrect').fill('lightblue');
				layer.draw();
			});

			// (3) render the arrows
			var getClosestPoint = (group1, group2) => {
				var cx = group1.x();
				var cy = group1.y();
				var width = group1.mywidth;
				var height = group1.myheight;
				var p1 = [
					[cx, cy-height/2],
					[cx, cy+height/2],
					[cx-width/2, cy],
					[cx+width/2, cy],
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
					var p1 = getClosestPoint(groups[src], groups[dst]);
					var p2 = getClosestPoint(groups[dst], groups[src]);
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

			var save = () => {
				var meta = {};
				for(var gid in groups) {
					meta[gid] = [parseInt(groups[gid].x()), parseInt(groups[gid].y())];
				}
				var params = {
					ID: this.selectedQuery,
					Meta: meta,
				};
				$.ajax({
					type: "POST",
					url: '/queries/render-meta',
					data: JSON.stringify(params),
					processData: false,
				});
			};

			// (4) add listeners to move the arrows when groups are dragged
			for(let gid in arrows) {
				let l = arrows[gid];
				groups[gid].on('dragmove', function() {
					l.forEach(function(el) {
						let mode = el[0];
						let arrow = el[1];
						let other = el[2];
						let p1 = getClosestPoint(groups[gid], groups[other]);
						let p2 = getClosestPoint(groups[other], groups[gid]);
						let points;
						if(mode == 'src') {
							points = [p1[0], p1[1], p2[0], p2[1]];
						} else {
							points = [p2[0], p2[1], p1[0], p1[1]];
						}
						arrow.points(points);
						layer.draw();
					});
				});
				groups[gid].on('dragend', save);
			}

			layer.draw();
		},
		onNewNodeModalClosed: function() {
			this.showNewNodeModal = false;
			this.update();
		},
		editNode: function() {
			if(this.selectedNode.Ext == 'python') {
				this.editor = 'node-edit-text';
			} else {
				this.editor = 'node-edit-' + this.selectedNode.Ext;
			}
		},
		backFromEditing: function() {
			this.editor = '';
			this.update();
		},
	},
	watch: {
		tab: function() {
			if(this.tab != '#queries-panel') {
				return;
			}
			this.fetchQueries(true);
		},
	},
	template: `
<div style="height: 100%">
	<div v-if="editor == ''" id="q-div">
		<div id="q-view" ref="view">
			<form v-on:submit.prevent="createQuery" class="form-inline my-2">
				<label class="ml-1">Query:</label>
				<select v-model="selectedQuery" @change="update" class="form-control ml-1">
					<option v-for="query in queries" :value="query.ID">{{ query.Name }}</option>
				</select>
				<button type="button" class="btn btn-danger ml-1">Remove</button>
				<input v-model="newQueryName" type="form-control" placeholder="Name" class="ml-4" />
				<button type="submit" class="btn btn-primary ml-1">Add Query</button>
			</form>
			<div ref="layer" style="position: absolute"></div>
		</div>
		<div>
			<div>
				<button type="button" class="btn btn-primary" v-on:click="showNewNodeModal = true">New Node</button>
				<button type="button" class="btn btn-primary" :disabled="selectedNode == null" v-on:click="editNode">Edit Node</button>
			</div>
		</div>
		<new-node-modal v-if="showNewNodeModal && selectedQuery != ''" :query_id="selectedQuery" v-on:closed="onNewNodeModalClosed"></new-node-modal>
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
