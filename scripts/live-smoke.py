import os, sys, json, time, socket, shutil, signal, subprocess, tempfile, urllib.request, urllib.error, http.cookiejar
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
TMP=Path(tempfile.mkdtemp(prefix='zyvor-fleet-live-'))
port_sock=socket.socket(); port_sock.bind(('127.0.0.1',0)); PORT=port_sock.getsockname()[1]; port_sock.close()
BASE=f'http://127.0.0.1:{PORT}'
server_log=open(TMP/'server.log','w')
agent_log=open(TMP/'agent.log','w')
server=None; agent=None

def start_server():
    global server, server_log
    env=os.environ.copy(); env.update({
        'ZYVOR_FLEET_ADMIN_PASSWORD':'live-smoke-password',
        'ZYVOR_FLEET_SESSION_SECRET':'live-smoke-session-secret-0123456789abcdef0123456789',
    })
    server=subprocess.Popen([str(ROOT/'bin/fleetd'),'--demo','--listen',f'127.0.0.1:{PORT}','--data',str(TMP/'state.json')],cwd=ROOT,env=env,stdout=server_log,stderr=subprocess.STDOUT)
    wait_url('/healthz', expect=200, timeout=10)

def wait_url(path, expect=200, timeout=10):
    end=time.time()+timeout
    while time.time()<end:
        try:
            with urllib.request.urlopen(BASE+path,timeout=1) as r:
                if r.status==expect: return
        except Exception: pass
        time.sleep(.1)
    raise RuntimeError(f'timeout waiting {path}')

def wait_for(fn, timeout=15, label='condition'):
    end=time.time()+timeout; last=None
    while time.time()<end:
        try:
            v=fn(); last=v
            if v: return v
        except Exception as e: last=e
        time.sleep(.25)
    raise RuntimeError(f'timeout waiting for {label}; last={last!r}')

jar=http.cookiejar.CookieJar(); opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
def api(method,path,body=None):
    data=None if body is None else json.dumps(body).encode()
    req=urllib.request.Request(BASE+path,data=data,method=method)
    if data is not None: req.add_header('Content-Type','application/json')
    if method not in ('GET','HEAD'): req.add_header('X-Zyvor-Request','1')
    try:
        with opener.open(req,timeout=4) as r:
            raw=r.read(); return r.status, json.loads(raw) if raw else None
    except urllib.error.HTTPError as e:
        raw=e.read().decode(); raise RuntimeError(f'{method} {path}: {e.code} {raw}')

def login():
    return api('POST','/api/v1/auth/login',{'email':'admin@zyvor.local','password':'live-smoke-password'})

def sites(): return api('GET','/api/v1/sites')[1]
def rollouts(): return api('GET','/api/v1/rollouts')[1]

def create_revision(name, manifest, health=None):
    w={'kind':'k3s','name':'edge-config','state':'running','manifest':manifest}
    if health: w['health']=health
    return api('POST','/api/v1/revisions',{'name':name,'workloads':[w]})[1]

def create_rollout(name, rev, site_ids=None, group_id='', auto=False):
    body={'name':name,'revisionId':rev['id'],'waveSize':1,'pauseSeconds':0,'maxFailures':0,'siteIds':site_ids or [],'groupId':group_id,'autoRollback':auto}
    return api('POST','/api/v1/rollouts',body)[1]

try:
    start_server(); login()
    env=os.environ.copy(); env.update({
        'ZYVOR_FLEET_K3S_MANIFEST_DIR':str(TMP/'manifests'),
        'ZYVOR_FLEET_ENROLLMENT_TOKEN':'zf_enroll_demo-local-only',
    })
    agent=subprocess.Popen([str(ROOT/'bin/fleet-agent'),'--server',BASE,'--name','live-factory-01','--region','test-west','--state',str(TMP/'agent.json'),'--labels','env=prod,class=factory','--interval','300ms'],cwd=ROOT,env=env,stdout=agent_log,stderr=subprocess.STDOUT)
    site=wait_for(lambda: sites()[0] if sites() else None,label='site enrollment')
    site_id=site['id']
    group=api('POST','/api/v1/site-groups',{'name':'Production factories','selector':{'env':'prod','class':'factory'}})[1]['group']
    plan=api('POST','/api/v1/rollouts/plan',{'groupId':group['id'],'waveSize':1})[1]
    assert plan['total']==1 and plan['waves']==1

    manifest1='apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: edge-config\ndata:\n  release: v1\n'
    rev1=create_revision('baseline',manifest1)
    ro1=create_rollout('baseline rollout',rev1,[site_id])
    wait_for(lambda: next((r for r in rollouts() if r['id']==ro1['id'] and r['status']=='completed'),None),label='baseline rollout completed')
    wait_for(lambda: next((s for s in sites() if s['id']==site_id and s.get('appliedRevision')==rev1['id']),None),label='baseline applied')
    manifest_path=TMP/'manifests'/'zyvor-fleet-edge-config.yaml'
    wait_for(lambda: manifest_path.exists() and 'release: v1' in manifest_path.read_text(),label='baseline manifest')

    # Closed localhost port makes the health gate fail deterministically.
    manifest2='apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: edge-config\ndata:\n  release: v2\n'
    rev2=create_revision('bad-canary',manifest2,{'type':'http','url':'http://127.0.0.1:1/healthz','timeoutSeconds':1,'graceSeconds':0})
    ro2=create_rollout('guarded canary',rev2,group_id=group['id'],auto=True)
    failed=wait_for(lambda: next((r for r in rollouts() if r['id']==ro2['id'] and r['status']=='failed' and r.get('rolledBack')),None),timeout=15,label='automatic rollback')
    restored=wait_for(lambda: next((s for s in sites() if s['id']==site_id and s.get('desiredRevision')==rev1['id'] and s.get('appliedRevision')==rev1['id']),None),timeout=15,label='rollback re-applied')
    wait_for(lambda: manifest_path.exists() and 'release: v1' in manifest_path.read_text(),label='rollback manifest restored')

    # WAN/control-plane outage: delete managed state and require cached local revision to restore it.
    server.send_signal(signal.SIGTERM); server.wait(timeout=5); server=None
    time.sleep(.8)
    manifest_path.unlink()
    wait_for(lambda: manifest_path.exists() and 'release: v1' in manifest_path.read_text(),timeout=6,label='offline cached drift repair')

    # Reconnect with exact same persisted control-plane state.
    server_log.flush(); server_log.close(); server_log=open(TMP/'server-restart.log','w')
    start_server(); login()
    wait_for(lambda: next((s for s in sites() if s['id']==site_id and s.get('status') in ('online','degraded')),None),timeout=10,label='agent reconnect')
    events=api('GET','/api/v1/events?limit=100')[1]
    kinds=[e.get('kind') for e in events]
    wait_for(lambda: ('controlplane.unreachable' in [e.get('kind') for e in api('GET','/api/v1/events?limit=100')[1]] and 'controlplane.reconnected' in [e.get('kind') for e in api('GET','/api/v1/events?limit=100')[1]]),timeout=10,label='queued reconnect events')
    metrics=urllib.request.urlopen(BASE+'/metrics',timeout=2).read().decode()
    assert 'zyvor_fleet_sites' in metrics

    print(json.dumps({
        'ok':True,'port':PORT,'siteId':site_id,'groupId':group['id'],'plan':plan,
        'baselineRollout':ro1['id'],'failedRollout':failed['id'],'autoRollback':failed.get('rolledBack'),
        'offlineRepair':True,'reconnected':True,'metrics':True
    },indent=2))
finally:
    if agent and agent.poll() is None:
        agent.send_signal(signal.SIGTERM)
        try: agent.wait(timeout=5)
        except: agent.kill()
    if server and server.poll() is None:
        server.send_signal(signal.SIGTERM)
        try: server.wait(timeout=5)
        except: server.kill()
    try: agent_log.flush(); agent_log.close()
    except: pass
    try: server_log.flush(); server_log.close()
    except: pass
    if os.environ.get('ZYVOR_FLEET_KEEP_SMOKE') != '1':
        shutil.rmtree(TMP, ignore_errors=True)
