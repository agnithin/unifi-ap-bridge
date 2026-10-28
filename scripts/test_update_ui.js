// Run the actual ASP inline script with a small browser/dispatcher test double.
const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const html = fs.readFileSync(require('node:path').join(__dirname, '../webui/unifi-bridge.asp'), 'utf8');
const script = [...html.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/gi)].map(m => m[1]).join('\n').replace(/<%[\s\S]*?%>/g, '10.100.1.1');
let clock = 1000;
let reloads = 0;
const timers = new Map();
const timeouts = [];
const scripts=[];
const nodes = {};
function makeNode() {
 const n={value:'', _textContent:'', style:{}, disabled:false, children:[], appendChild(child){this.children.push(child);}};
 Object.defineProperty(n,'textContent',{get(){return this._textContent;},set(value){this._textContent=String(value);this.children=[];}});
 return n;
}
function node(id) { return nodes[id] ||= makeNode(); }
function submittedSettings() {
 const envelope=JSON.parse(node('amng_custom').value);
 assert.deepEqual(Object.keys(envelope),['unifibridge_payload']);
 assert.ok(envelope.unifibridge_payload.length < 2999,'custom setting value must fit Merlin storage');
 assert.ok('unifibridge_payload'.length <= 29,'custom setting key must fit Merlin storage');
 return JSON.parse(envelope.unifibridge_payload);
}
const form = {action:'/start_apply.htm', elements:[], submit() {throw new Error('native iframe submit is forbidden for update checks');}};
for (const name of ['action_script','action_mode','action_wait','current_page','next_page','unifibridge_branch','amng_custom']) {
 const field = {name, value:'', type:'hidden'}; form[name]=field; form.elements.push(field);
 if (name==='amng_custom') nodes[name]=field;
}
const requests=[];
function XHR() {requests.push(this); this.status=200;}
XHR.prototype.open=function(method,url,async){this.method=method;this.url=url;this.async=async;};
XHR.prototype.setRequestHeader=function(){};
XHR.prototype.send=function(body){this.body=body;};
const context = {
 document:{form, getElementById:node, createElement(){return makeNode();}, head:{appendChild(s){scripts.push(s);if(s.onload)s.onload();},removeChild(){}}}, XMLHttpRequest:XHR,
 window:{parent:{},location:{reload(){reloads++;}}},
 Date:{now:()=>clock},
 setInterval(fn){const id=timers.size+1;timers.set(id,fn);return id;},
 clearInterval(id){timers.delete(id);},setTimeout(fn){timeouts.push(fn);},
 confirm:()=>true, console,
};
vm.createContext(context);vm.runInContext(script,context);
context.installedVersion='1.2.3';context.currentBranch='dev';
node('select_branch').value='main';
context.onTrackChanged();
assert.equal(context.currentBranch,'dev','selection must not relabel the installed track');
assert.equal(requests[0].method,'POST');
assert.equal(requests[0].async,true);
const firstUpdateId=context.activeUpdateRequest.id;
assert.match(firstUpdateId,/^[0-9]{4,24}$/);
assert.match(requests[0].body,new RegExp('action_script=start_unifi_bridge_check_update_main_'+firstUpdateId));
assert.equal(node('update_progress_box').style.display,'none');
assert.equal(node('btn_check_update').disabled,true);
const result={status:'UP_TO_DATE',branch:'main',operation:'check',request_id:firstUpdateId,current_version:'1.2.3',remote_version:'v1.2.1',installed_branch:'dev',installed_newer:true};
context.updateUpdateUI({...result, request_id:'old',status:'SUCCESS'});
assert.ok(context.activeUpdateRequest,'old result cannot finish current request');
assert.equal(timeouts.length,0,'old success cannot reload a check');
context.updateUpdateUI(result);
assert.equal(node('cur_ver_val').textContent,'v1.2.3 (dev)');
assert.equal(node('rem_ver_val').textContent,'Latest (main): v1.2.1');
assert.equal(node('update_badge').textContent,'INSTALLED IS NEWER');
assert.equal(context.updatePollingTimer,null);
assert.equal(context.activeUpdateRequest,null);
assert.equal(node('btn_check_update').disabled,false);
assert.equal(timeouts.length,0,'completed check must not reload');
clock=2000;context.checkUpdateNow();
clock=48001;[...timers.values()][0]();
assert.equal(context.activeUpdateRequest,null);
assert.match(node('update_desc').textContent,/timed out/);
clock=50000;context.checkUpdateNow();requests.at(-1).onerror();
assert.match(node('update_desc').textContent,/Could not contact/);
assert.equal(node('btn_check_update').disabled,false);
clock=60000;node('select_branch').value='dev';context.doUpdateNow();
const applyUpdateId=context.activeUpdateRequest.id;
assert.equal(node('update_progress_box').style.display,'block');
context.updateUpdateUI({...result,branch:'dev',operation:'apply',request_id:applyUpdateId,status:'SUCCESS'});
assert.equal(timeouts.length,1,'only matching install success may reload');
timeouts[0]();assert.equal(reloads,1);
assert.equal(context.displayVersion('v1.2.3'),'v1.2.3');
assert.equal(context.displayVersion('1.2.3'),'v1.2.3');
console.log('Update UI regressions passed: background POST, track/version labels, stale responses, timeout, errors, install-only reload.');

assert.equal(context.clusterState({status:'RUNNING',sync_status:'Local'}),'local');
assert.equal(context.clusterState({status:'RUNNING',sync_status:'In Sync'}),'verified');
assert.equal(context.clusterState({status:'RUNNING',sync_status:'Version Match (Config Unverified)'}),'unverified');
assert.equal(context.clusterState({status:'RUNNING',sync_status:'Config Synced'}),'unverified');
assert.equal(context.clusterState({status:'STOPPED',sync_status:'Stale or Stopped'}),'error');
assert.equal(context.clusterState({status:'STOPPED',enabled:false,running:false,freshness:'fresh',pid:null,sync_status:'Disabled'}),'disabled');
assert.equal(context.clusterState({status:'STOPPED',enabled:false,running:false,freshness:'fresh',pid:'123',sync_status:'Disabled'}),'error');
assert.equal(context.escapeHTML('<img onerror="x">'), '&lt;img onerror=&quot;x&quot;&gt;');
clock=1000000;
assert.equal(context.statusIsFresh({status_updated_at:990,interval_sec:10}),true);
assert.equal(context.statusIsFresh({status_updated_at:900,interval_sec:10}),false);
assert.equal(context.statusIsFresh({running:true}),false);
console.log('Health UI regressions passed: stale status, unverified cluster state, escaped text.');

context.updateClusterUI({nodes:[{name:'<img src=x onerror=1>',role:'Satellite Node',version:'1.2.5',commit:'a'.repeat(40),ip:'10.0.0.2',status:'RUNNING',enabled:true,running:true,freshness:'fresh',outcome:'success',stage:'verified',dispatch_result:'succeeded',sync_status:'<script>x</script>',total_clients:'<img src=x>',clients_2g:'<svg onload=x>',clients_5g:0,cu_2g:'<img src=x>',cu_5g:0}]});
assert.ok(!node('cluster_nodes_tbody').innerHTML.includes('<img'));
assert.ok(node('cluster_nodes_tbody').innerHTML.includes('&lt;img'));
assert.ok(!node('cluster_nodes_tbody').innerHTML.includes('<script>'));
assert.ok(!node('cluster_nodes_tbody').innerHTML.includes('<svg'));
assert.ok(node('cluster_nodes_tbody').innerHTML.includes('-G / 0A'));
assert.equal(node('cluster_sync_badge').textContent,'HEALTH OK · CONFIGURATION UNVERIFIED');
context.updateClusterUI({nodes:[
  {name:'Main',role:'Main Gateway',status:'RUNNING',running:true,freshness:'fresh',sync_status:'Local',
   ports:[{name:'Port 1',ifname:'eth0',up:true,speed:2500,full_duplex:true,is_uplink:true,last_connection:{ip:'10.100.1.1'}}]},
  {name:'Node',role:'Satellite Node',status:'RUNNING',running:true,freshness:'fresh',sync_status:'In Sync',
   ports:[{name:'Port 4',ifname:'eth3',up:true,speed:1000,full_duplex:true,is_uplink:false,mac_table:[{mac:'10:2c:b1:73:3d:00',vlan:20}]}]}
]});
assert.equal(node('cluster_sync_badge').textContent,'HEALTH OK · IN SYNC');
assert.ok(node('cluster_ports_tbody').innerHTML.includes('Main'));
assert.ok(node('cluster_ports_tbody').innerHTML.includes('Node'));
assert.ok(node('cluster_ports_tbody').innerHTML.includes('VLAN 20'));
assert.ok(node('cluster_ports_tbody').innerHTML.includes('10:2c:b1:73:3d:00'));
context.updateClusterUI({nodes:[{name:'Main',role:'Main',status:'RUNNING',running:true,freshness:'stale',sync_status:'Local'}]});
assert.equal(node('cluster_sync_badge').textContent,'CONFIGURATION UNVERIFIED','stale Main health must not be labeled healthy');
context.updateClusterUI({nodes:[]});
assert.equal(node('cluster_sync_badge').textContent,'CONFIGURATION UNVERIFIED','empty node data must not be labeled healthy');
console.log('Cluster rendering regression passed: text and metrics are safe; health badge makes no config equality claim.');

// Applying unrelated settings after checking main must retain installed dev.
for (const name of ['unifibridge_enabled','unifibridge_auto_gateway','unifibridge_controller_url','unifibridge_model','unifibridge_interval']) form[name]={value:''};
form.submit=()=>{};
node('unifibridge_enabled_1').checked=true;
node('unifibridge_auto_1').checked=false;
node('input_controller_url').value='http://controller.test:8080/inform';
node('select_model').value='U6IW';node('select_interval').value='10';
node('select_branch').value='main';context.currentBranch='dev';
context.applySettings();
assert.equal(submittedSettings().branch,'dev');
context.finishActionRequest();
console.log('Settings track regression passed: checking main does not change installed dev on save.');

context.formInitialized=false;
context.initFormOnce({enabled:false,branch:'dev',model_code:'U6IW',interval_sec:10});
assert.equal(node('unifibridge_enabled_1').checked,false);
assert.equal(node('unifibridge_enabled_0').checked,true);
console.log('Disabled UI regression passed: unrelated edits do not silently re-enable the service.');
context.detectedGatewayIP='10.100.1.1';
context.formInitialized=false;
context.initFormOnce({auto_gateway:false,controller_url:'http://10.100.1.1:8080/inform'});
assert.equal(node('unifibridge_auto_1').checked,false,'explicit manual mode must survive a URL matching the gateway');
assert.equal(node('unifibridge_auto_0').checked,true);
assert.equal(node('input_controller_url').value,'http://10.100.1.1:8080/inform');
context.formInitialized=false;
context.initFormOnce({auto_gateway:true,controller_url:''});
assert.equal(node('unifibridge_auto_1').checked,true,'explicit auto mode must initialize even with an empty URL');
console.log('Controller mode regression passed: explicit saved mode takes precedence over URL inference.');


context.updateUI({running:true,pid:123,status_updated_at:1000,sample_fresh:false,sample_age_sec:12,enabled:true,branch:'dev',model_code:'U6IW',interval_sec:10});
assert.match(node('bridge_status_text').textContent,/Telemetry sample stale \(12s old\)/);
context.updateUI({running:true,pid:123,status_updated_at:1000,sample_fresh:true,sample_stage_errors:{ports:'unavailable'},enabled:true,branch:'dev',model_code:'U6IW',interval_sec:10});
assert.match(node('bridge_status_text').textContent,/Partial telemetry/);
console.log('Sampling status regression passed: stale and partial telemetry remain explicit.');

context.updateUI({running:true,pid:123,status_updated_at:1000,sample_fresh:true,enabled:true,branch:'dev',model_code:'U6IW',interval_sec:10,
 ports:[{name:'<img src=x onerror=1>',up:true,speed:'1000<script>',is_uplink:true},{name:'eth0',up:true,speed:2500,is_uplink:false},{name:'eth1',up:false,speed:1000}]});
const renderedPorts=node('ports_val').children[0].children.map(b=>b.textContent);
assert.deepEqual(renderedPorts,['<img src=x onerror=1>: Speed unknown UPLINK','eth0: 2500M FDX','eth1: DOWN']);
assert.equal(node('ports_val').children[0].children[0].innerHTML,undefined,'port names must be assigned as text, never markup');
context.updateUI({running:true,pid:123,status_updated_at:1000,sample_fresh:true,enabled:true,branch:'dev',model_code:'U6IW',interval_sec:10,
 ports:[{name:'eth0',up:true,speed:Infinity,is_uplink:false}]});
assert.equal(node('ports_val').children[0].children[0].textContent,'eth0: Speed unknown FDX','malformed numeric speeds must be explicit and safe');
console.log('Port UI regression passed: labels use text nodes and malformed speeds are rejected.');

context.detectedGatewayIP='';
context.updateUI({running:true,pid:123,status_updated_at:1000,sample_fresh:true,enabled:true,branch:'dev',model_code:'U6IW',interval_sec:10,controller_url:''});
assert.equal(node('controller_val').textContent,'Unknown','missing gateway must remain unknown in status');
for (const invalidGateway of ['not-an-ip','0.0.0.0','127.0.0.1','224.0.0.1','255.255.255.255','2001:db8::1']) {
  context.detectedGatewayIP=invalidGateway;
  assert.equal(context.detectedGatewayURL(),'','non-unicast gateway must not be used as a controller URL');
}
context.detectedGatewayIP='';
node('unifibridge_auto_1').checked=true;
context.toggleControllerInput();
assert.equal(node('input_controller_url').value,'','auto mode must not synthesize a controller URL without a gateway');
console.log('Gateway fallback regression passed: missing gateway stays unknown.');

context.applySettings();
assert.equal(form.unifibridge_controller_url.value,'','saving auto mode without a detected gateway must preserve unknown');
assert.equal(submittedSettings().controller_url,'');
context.finishActionRequest();
console.log('Gateway settings regression passed: auto mode does not invent an endpoint.');

clock=70000;context.applySettings();
const settingsRequest=context.activeActionRequest;
assert.ok(settingsRequest);
assert.match(form.action_script.value,new RegExp('start_unifi_bridge_apply_'+settingsRequest.id));
assert.equal(submittedSettings().request_id,settingsRequest.id);
assert.equal(requests.at(-1).async,true,'settings must use background POST');
assert.equal(form.submit(),undefined,'background settings must not submit the page iframe');
context.syncClusterNow();
assert.equal(context.activeActionRequest,settingsRequest,'overlap must be rejected while settings are pending');
context.window.unifiBridgeAction={request_id:settingsRequest.id,operation:'settings',status:'SAVED',message:'saved locally; runtime unverified'};
clock=71500;[...timers.values()].at(-1)();
assert.equal(context.activeActionRequest,null);
assert.match(node('action_status').textContent,/runtime unverified/);

clock=80000;context.syncClusterNow();
const syncRequest=context.activeActionRequest;
assert.match(form.action_script.value,new RegExp('start_unifi_bridge_sync_'+syncRequest.id));
context.window.unifiBridgeOperation={request_id:syncRequest.id,operation:'sync',status:'PARTIAL',completed_at:1234,
 message:'sync completed with one node failure',nodes:[]};
clock=81500;[...timers.values()].at(-1)();
assert.equal(context.activeActionRequest,null);
assert.match(node('action_status').textContent,/one node failure/);

clock=90000;context.applySettings();
const unknownRequest=context.activeActionRequest;
const postedCount=requests.length;
clock=150001;[...timers.values()].at(-1)();
assert.equal(context.activeActionRequest,null);
assert.match(node('action_status').textContent,/completion was not confirmed/);
assert.equal(requests.length,postedCount,'unknown completion must not trigger automatic resubmission');
assert.ok(unknownRequest);
console.log('Action UI regressions passed: request IDs, background settings/sync POST, matching results, overlap rejection and unknown timeout without retry.');

// Diagnostic data is rendered as text, including untrusted SSIDs/client labels.
context.updateDiagnosticsUI({
 radio_stats:[{name:"wifi-na-1",channel:161,bw:80,cu_total:35,cu_obss:12,cu_nocat:8,cu_nopkt:6,cca_interference:0,noise_dbm:-91}],
 passive_rf_trends:[{radio:"wifi-na-1",mean_busy_percent:30}],
 station_events:[{type:"observed_radio_change",observed_at:"2026-10-26T00:00:00Z",mac:"fixture-client",essid:"<img onerror=1>",physical_radio:"wifi-na-1",previous_physical_radio:"wifi-na",signal:-65,signal_known:true,poll_gap_seconds:10}],
 wlan_mapping:{entries:2,file_enabled:true},
 controller_commands:{reboot_enabled:false,last_observation:{action:"locate"}}
});
assert.equal(node("rf_diagnostics_rows").children[0].children[2].textContent,"80 MHz");
assert.equal(node("rf_diagnostics_rows").children[0].children[5].textContent,"8%");
assert.equal(node("rf_diagnostics_rows").children[0].children[6].textContent,"6%");
assert.equal(node("rf_diagnostics_rows").children[0].children[7].textContent,"0%","measured zero CCA interference must remain visible");
assert.equal(node("station_event_rows").children[0].children[3].textContent,"<img onerror=1>");
assert.equal(node("station_event_rows").children[0].children[3].children.length,0);
assert.match(node("controller_command_status").textContent,/Disabled/);
assert.match(node("wlan_mapping_status").textContent,/2 SSID mappings/);
context.updateDiagnosticsUI({radio_stats:[{name:"wifi-na-1",channel:161,bw:80,cu_total:35}]});
assert.equal(node("rf_diagnostics_rows").children[0].children[5].textContent,"-","unknown CCA category must not render as zero");
assert.equal(node("rf_diagnostics_rows").children[0].children[7].textContent,"-","unknown CCA interference must render as dash");
console.log("RF and station diagnostic rendering passed: actual width, bounded text-only rows and opt-in command status.");
