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
function node(id) { return nodes[id] ||= {value:'', textContent:'', style:{}, disabled:false}; }
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
 document:{form, getElementById:node, createElement(){return {};}, head:{appendChild(s){scripts.push(s);if(s.onload)s.onload();},removeChild(){}}}, XMLHttpRequest:XHR,
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
assert.equal(JSON.parse(node('amng_custom').value).branch,'dev');
context.finishActionRequest();
console.log('Settings track regression passed: checking main does not change installed dev on save.');

context.formInitialized=false;
context.initFormOnce({enabled:false,branch:'dev',model_code:'U6IW',interval_sec:10});
assert.equal(node('unifibridge_enabled_1').checked,false);
assert.equal(node('unifibridge_enabled_0').checked,true);
console.log('Disabled UI regression passed: unrelated edits do not silently re-enable the service.');

context.updateUI({running:true,pid:123,status_updated_at:1000,sample_fresh:false,sample_age_sec:12,enabled:true,branch:'dev',model_code:'U6IW',interval_sec:10});
assert.match(node('bridge_status_text').textContent,/Telemetry sample stale \(12s old\)/);
context.updateUI({running:true,pid:123,status_updated_at:1000,sample_fresh:true,sample_stage_errors:{ports:'unavailable'},enabled:true,branch:'dev',model_code:'U6IW',interval_sec:10});
assert.match(node('bridge_status_text').textContent,/Partial telemetry/);
console.log('Sampling status regression passed: stale and partial telemetry remain explicit.');

clock=70000;context.applySettings();
const settingsRequest=context.activeActionRequest;
assert.ok(settingsRequest);
assert.match(form.action_script.value,new RegExp('start_unifi_bridge_apply_'+settingsRequest.id));
assert.equal(JSON.parse(node('amng_custom').value).request_id,settingsRequest.id);
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
