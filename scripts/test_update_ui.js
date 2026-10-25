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
 document:{form, getElementById:node}, XMLHttpRequest:XHR,
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
assert.match(requests[0].body,/action_script=start_unifi_bridge_check_update_main_1000/);
assert.equal(node('update_progress_box').style.display,'none');
assert.equal(node('btn_check_update').disabled,true);
const result={status:'UP_TO_DATE',branch:'main',operation:'check',request_id:'1000',current_version:'1.2.3',remote_version:'v1.2.1',installed_branch:'dev',installed_newer:true};
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
assert.equal(node('update_progress_box').style.display,'block');
context.updateUpdateUI({...result,branch:'dev',operation:'apply',request_id:'60000',status:'SUCCESS'});
assert.equal(timeouts.length,1,'only matching install success may reload');
timeouts[0]();assert.equal(reloads,1);
assert.equal(context.displayVersion('v1.2.3'),'v1.2.3');
assert.equal(context.displayVersion('1.2.3'),'v1.2.3');
console.log('Update UI regressions passed: background POST, track/version labels, stale responses, timeout, errors, install-only reload.');
