async page => {
  const base='http://127.0.0.1:18174', out='output/playwright';
  await page.unrouteAll({behavior:'ignoreErrors'});
  const settings={'viewer.performance.mode':'quality','auth.session_hours':'12','auth.viewer_public':'false','auth.local_admin_bypass':'false','network.lan_access_enabled':'true','camera.relay.ids':'test','camera.relay.test.name':'Test relay','camera.relay.test.ssh_target':'relay-host'};
  const auth={enabled:true,role:'admin',authenticated:true,admin_password_set:true,viewer_password_set:true,session_hours:12};
  const version={version:'0.5.0',commit:'63f2651',status:'ok'};
  const devices=[{id:'courtyard',manufacturer:'Test',model:'Hofkamera',last_ip:'192.0.2.10',raw_json:{rtsp_port_open:true}}];
  const identities=[{id:'shared',name:'Gemeinsamer Zugang',username:'viewer',password_set:true}];
  const events=Array.from({length:8},(_,i)=>({id:String(i),created_at:'2026-09-06T08:00:00Z',type:'camera.connected',level:'info',message:i===0?'Kameraverbindung wiederhergestellt.':`Fixture event ${i+1}`}));
  let saveCount=0,failBundle=true,bundlePosts=0,updateChecks=0,updateReads=0;
  let updatePhase='up_to_date';
  await page.route(base+'/api/**',async route=>{
    const path=route.request().url().slice(base.length).split('?')[0],method=route.request().method();
    let data=[];
    if(path==='/api/auth/status')data=auth;
    else if(path==='/api/health')data=version;
    else if(path==='/api/settings'){if(method==='PUT'){Object.assign(settings,route.request().postDataJSON());saveCount++}data=settings}
    else if(path==='/api/status')data={devices,slots:[],bindings:[],relays:[],system:{camera_appliance:{online:true},go2rtc:{online:true}},watchdog:{enabled:false},version,recent_events:events,scan_runs:[]};
    else if(path==='/api/system/update/status'){updateReads++;data={phase:updatePhase,current_version:'0.5.1'}}
    else if(path==='/api/system/update/check'){updateChecks++;data={phase:'up_to_date',current_version:'0.5.1'}}
    else if(path==='/api/credential-identities')data=identities;
    else if(path==='/api/devices')data=devices;
    else if(path==='/api/devices/courtyard')data=devices[0];
    else if(path.endsWith('/credentials'))data={username:'viewer',password_set:true};
    else if(path==='/api/snapshot-upload')data={protocol:'sftp',host:'images.example.test',port:22,username:'uploads',directory:'.',password_set:true};
    else if(path==='/api/events')data=events;
    else if(path==='/api/support')data={version,events};
    else if(path==='/api/support-bundle/download'){
      bundlePosts++;
      if(failBundle){failBundle=false;await route.fulfill({status:500,json:{error:'Test: Erstellung fehlgeschlagen'}});return}
      await route.fulfill({contentType:'application/gzip',body:'fixture-download'});return
    }
    else if(path.includes('reference-image')){await route.fulfill({status:404});return}
    await route.fulfill({json:data});
  });
  await page.setViewportSize({width:1440,height:1000});
  await page.goto(base+'/verwaltung');
  await page.waitForURL(base+'/einrichtung');
  if(await page.locator('.rail').getByRole('link',{name:'Home',exact:true}).count())throw Error('Desktop has Home');
  await page.getByRole('button',{name:'Kamera hinzufügen',exact:true}).locator('svg').waitFor();
  await page.goto(base+'/system/allgemein');
  const summary=page.getByRole('paragraph').filter({hasText:/^Qualität$/});
  await summary.click();
  let dialog=page.getByRole('dialog',{name:'Anzeige bearbeiten',exact:true});
  await dialog.waitFor();
  await dialog.getByRole('combobox').selectOption('balanced');
  await dialog.getByRole('button',{name:'Abbrechen',exact:true}).click();
  await page.getByRole('dialog',{name:'Änderungen verwerfen?'}).getByRole('button',{name:'Verwerfen',exact:true}).click();
  await dialog.waitFor({state:'hidden'});
  if(saveCount!==0)throw Error('Cancel saved settings');
  // Text selection must not activate the section.
  await summary.evaluate(e=>{const r=document.createRange();r.selectNodeContents(e);window.getSelection().removeAllRanges();window.getSelection().addRange(r);e.dispatchEvent(new MouseEvent('click',{bubbles:true}));});
  if(await dialog.isVisible())throw Error('Selection opens editor');
  await page.evaluate(()=>window.getSelection().removeAllRanges());
  await page.getByRole('button',{name:'Anzeige bearbeiten',exact:true}).focus();
  await page.keyboard.press('Enter');
  await dialog.waitFor();
  await dialog.getByRole('combobox').selectOption('balanced');
  await dialog.getByRole('button',{name:'Anzeige speichern'}).click();
  await dialog.waitFor({state:'hidden'});
  if(saveCount!==1 || settings['viewer.performance.mode']!=='balanced')throw Error('Keyboard/save failed');
  await page.goto(base+'/system/zugriff');
  if(await page.locator('.edit-section').count()!==3)throw Error('Access sections not grouped');
  await page.getByRole('button',{name:'Admin-Passwort ändern'}).click();
  await page.getByRole('dialog',{name:'Admin-Passwort bearbeiten',exact:true}).waitFor();
  if(await page.getByRole('dialog',{name:'Administration bearbeiten',exact:true}).isVisible())throw Error('Inner button opened section too');
  await page.getByRole('button',{name:'Schließen',exact:true}).click();
  await page.getByRole('region',{name:'Administration',exact:true}).getByRole('term').filter({hasText:/^Sitzungsdauer$/}).click();
  await page.getByRole('dialog',{name:'Administration bearbeiten',exact:true}).waitFor();
  await page.getByRole('button',{name:'Schließen',exact:true}).click();
  const active=page.locator('.rail .nav-active');
  if(await active.count()!==1 || !((await active.textContent()).includes('Zugriff')))throw Error('Ambiguous active row');
  const style=await active.evaluate(e=>({bg:getComputedStyle(e).backgroundColor,rail:getComputedStyle(e.closest('.rail')).backgroundColor,left:e.getBoundingClientRect().left,width:e.getBoundingClientRect().width,parent:e.parentElement.getBoundingClientRect().width}));
  if(style.bg===style.rail || style.bg==='rgba(0, 0, 0, 0)' || style.width<style.parent-2)throw Error('Active background does not fill row');
  if(await page.locator('.rail .nav a:not(:has(svg))').count())throw Error('Missing navigation icon');
  const metadata=await page.locator('.rail-foot .row').evaluateAll(rows=>rows.map(row=>({label:row.firstElementChild.textContent,left:row.firstElementChild.getBoundingClientRect().left,right:row.lastElementChild.getBoundingClientRect().right})));
  if(metadata.map(x=>x.label).join(',')!=='Stand,Login,Version' || new Set(metadata.map(x=>x.right)).size!==1)throw Error('Footer columns not aligned');
  await page.screenshot({path:out+'/access-desktop.png',fullPage:true,animations:'disabled'});
  await page.goto(base+'/kamera/courtyard');
  await page.getByRole('region',{name:'Kamera-Zugang',exact:true}).getByText('Benutzername',{exact:true}).click();
  await page.waitForURL('**/kamera/courtyard/bearbeiten#zugang');
  if(await page.locator('.rail .nav-active').count()!==1 || !((await page.locator('.rail .nav-active').textContent()).includes('Kameras')))throw Error('Camera detail not active');
  await page.goto(base+'/kameras/bild-upload');
  await page.getByText('images.example.test',{exact:true}).click();
  await page.waitForURL('**/kameras/bild-upload/bearbeiten');
  await page.goto(base+'/system/identitaeten/shared');
  await page.getByRole('region',{name:'Kamera-Login',exact:true}).getByText('Name',{exact:true}).click();
  await page.getByRole('dialog',{name:'Identität bearbeiten'}).waitFor();
  await page.getByRole('button',{name:'Schließen',exact:true}).click();
  await page.goto(base+'/system/wartung/support');
  await page.locator('.support-events li').first().waitFor();
  if(await page.locator('.support-events li').count()!==5)throw Error('Support preview must have only five events');
  await page.getByLabel('Was funktioniert nicht?').fill('Das Bild flackert & friert ein.');
  let href=await page.getByRole('link',{name:'Hilfe anfragen',exact:true}).getAttribute('href');
  if(!href.startsWith('mailto:mail@tbuck.de?') || decodeURIComponent(href).includes('Kameraverbindung'))throw Error('Mail recipient or diagnostics opt-in wrong');
  await page.getByLabel('Version und Protokollvorschau in die E-Mail aufnehmen').check();
  href=await page.getByRole('link',{name:'Hilfe anfragen',exact:true}).getAttribute('href');
  if(!decodeURIComponent(href).includes('Kameraverbindung wiederhergestellt.') || decodeURIComponent(href).includes('Fixture event 6'))throw Error('Mail should include only selected preview');
  const logDownload=page.waitForEvent('download');
  await page.getByRole('button',{name:'Protokoll herunterladen',exact:true}).click();
  if((await logDownload).suggestedFilename()!=='watchdeck-ereignisprotokoll.txt')throw Error('Log download missing');
  await page.getByRole('button',{name:'Diagnosepaket herunterladen',exact:true}).click();
  await page.getByRole('alert').filter({hasText:'Test: Erstellung fehlgeschlagen'}).waitFor();
  if(await page.getByLabel('Was funktioniert nicht?').inputValue()!=='Das Bild flackert & friert ein.')throw Error('Bundle failure lost description');
  const download=page.waitForEvent('download');
  await page.getByRole('button',{name:'Diagnosepaket herunterladen',exact:true}).click();
  if(!(await download).suggestedFilename().endsWith('.tar.gz') || bundlePosts!==2)throw Error('Download failed');
  await page.screenshot({path:out+'/support-desktop.png',fullPage:true,animations:'disabled'});
  await page.getByLabel('Was funktioniert nicht?').fill('');
  await page.goto(base+'/system/ueber');
  await page.getByRole('link',{name:/Buy Me a Coffee/}).waitFor();
  await page.getByText('MIT-Lizenz lesen',{exact:true}).click();
  await page.getByText('Copyright (c) 2026 Torben Buck',{exact:false}).waitFor();
  await page.screenshot({path:out+'/about-desktop.png',fullPage:true,animations:'disabled'});
  const checksBefore=updateChecks;
  const checked=page.waitForResponse(response=>response.url().endsWith('/api/system/update/check'));
  await page.getByRole('button',{name:'Nach Updates suchen',exact:true}).click();
  await checked;
  if(updateChecks!==checksBefore+1)throw Error('About update action missing');
  updatePhase='downloading';
  const readsBefore=updateReads;
  await page.goto(base+'/system/zugriff');
  await page.waitForTimeout(1300);
  if(updateReads<readsBefore+2)throw Error('Update monitoring stops outside About');
  updatePhase='up_to_date';
  const nav=page.getByRole('navigation',{name:'Hauptnavigation',exact:true});
  if(await nav.getByRole('link',{name:'Allgemein',exact:true}).count() || await nav.getByRole('link',{name:'Updates',exact:true}).count() || await nav.getByRole('link',{name:'Ereignisprotokoll',exact:true}).count())throw Error('Obsolete sidebar entries remain');
  if(await page.locator('.rail .rail-update').count())throw Error('Update trigger still in sidebar');
  for(const [path,current,parent] of [['/system','System',''],['/system/zugriff','Zugriff','System'],['/system/relays/test','Relays','System'],['/system/identitaeten/shared','Identitäten','System'],['/system/wartung','Wartung',''],['/system/wartung/watchdog','Watchdog','Wartung'],['/system/wartung/support','Support','Wartung'],['/system/ueber','Über Watchdeck','']]){
    await page.goto(base+path);await page.getByRole('heading',{level:1}).waitFor();
    const active=page.locator('.rail .nav-active');
    if(await active.count()!==1 || (await active.getAttribute('aria-current'))!=='page' || (await active.locator('span').first().textContent())!==current)throw Error('Current destination wrong: '+path);
    if(parent && (await page.locator('.rail .nav-parent-active span').first().textContent())!==parent)throw Error('Parent wrong: '+path);
    const appearance=await active.evaluate(e=>({radius:getComputedStyle(e).borderTopLeftRadius,bottom:getComputedStyle(e).borderBottomLeftRadius,border:getComputedStyle(e).borderLeftWidth,shadow:getComputedStyle(e).boxShadow}));
    if(appearance.radius!=='0px'||appearance.bottom!=='0px'||appearance.border!=='3px'||appearance.shadow!=='none')throw Error('Active edge not straight');
    if(await page.locator('.rail .nav svg:not(.lucide)').count())throw Error('Navigation contains custom icons');
  }
  await page.goto(base+'/system/wartung');
  const areas=page.getByRole('navigation',{name:'Wartungsbereiche'});
  if(await areas.getByRole('link').count()!==4)throw Error('Maintenance destinations incomplete');
  const sizes=await page.locator('.rail .nav a').evaluateAll(es=>es.map(e=>({child:!!e.closest('.nav-children'),size:parseFloat(getComputedStyle(e).fontSize)})));
  if(sizes.some(e=>e.size!==(e.child?13:14)))throw Error('Navigation typography does not show hierarchy');
  await page.screenshot({path:out+'/maintenance-desktop.png',fullPage:true,animations:'disabled'});
  await page.getByRole('navigation',{name:'Hauptnavigation',exact:true}).getByRole('link',{name:'System',exact:true}).focus();
  await page.keyboard.press('Enter');await page.waitForURL(base+'/system');
  await page.getByRole('heading',{level:1,name:'System',exact:true}).waitFor();
  await page.getByRole('navigation',{name:'Hauptnavigation',exact:true}).getByRole('link',{name:'Wartung',exact:true}).focus();
  await page.keyboard.press('Enter');await page.waitForURL(base+'/system/wartung');
  for(const [legacy,target] of [['/system/allgemein','/system'],['/system/wartung#updates','/system/ueber#updates'],['/system/wartung/ereignisse','/system/wartung/support#ereignisse']]){await page.goto(base+legacy);await page.waitForURL(base+target)}
  for(const [path,action,title] of [['/einrichtung','Kamera hinzufügen','Kamera per RTSP hinzufügen'],['/system/relays','Relay hinzufügen','Relay hinzufügen'],['/system/identitaeten','Identität hinzufügen','Identität hinzufügen']]){
    await page.goto(base+path);
    const header=page.locator('header.topline');
    const add=header.getByRole('button',{name:action,exact:true});await add.waitFor();
    const position=await add.boundingBox(),head=await header.boundingBox();if(Math.abs(position.x+position.width-head.x-head.width)>3)throw Error('Page action not right-aligned');
    await add.click();const dialog=page.getByRole('dialog',{name:title,exact:true});await dialog.waitFor();
    const submit=dialog.locator('button[type=submit]'),body=dialog.locator('.dialog-body');
    const buttonBox=await submit.boundingBox(),bodyBox=await body.boundingBox();if(Math.abs(buttonBox.x+buttonBox.width-(bodyBox.x+bodyBox.width-24))>3)throw Error('Modal actions not right-aligned');
    await page.screenshot({path:out+'/'+path.split('/').pop()+'-modal.png',fullPage:true,animations:'disabled'});
    await dialog.getByRole('button',{name:'Schließen',exact:true}).click();
  }
  for(const width of [390,320]){
    await page.setViewportSize({width,height:844});
    for(const path of ['/verwaltung','/system','/einrichtung','/system/wartung','/system/zugriff','/system/wartung/support','/system/ueber']){
      await page.goto(base+path);await page.getByRole('heading',{level:1}).waitFor();
      if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error(`Overflow at ${width}: ${path}`);
      if(width===390 && path!=='/verwaltung')await page.screenshot({path:out+'/'+path.split('/').pop()+'-mobile.png',fullPage:true,animations:'disabled'});
    }
  }
  for(const width of [390,320]) {
    await page.setViewportSize({width,height:844});await page.goto(base+'/einrichtung');
    await page.getByRole('button',{name:'Kamera hinzufügen',exact:true}).click();
    const dialog=page.getByRole('dialog',{name:'Kamera per RTSP hinzufügen'});await dialog.waitFor();
    const submit=await dialog.locator('button[type=submit]').boundingBox(),body=await dialog.locator('.dialog-body').boundingBox();
    if(Math.abs(submit.x+submit.width-(body.x+body.width-20))>3)throw Error('Mobile modal action not right-aligned');
    await page.screenshot({path:out+'/camera-modal-'+width+'.png',fullPage:true,animations:'disabled'});
    await dialog.getByRole('button',{name:'Schließen',exact:true}).click();
  }
  await page.goto(base+'/verwaltung');
  await page.getByRole('heading',{level:1,name:'Home'}).waitFor();
  await page.setViewportSize({width:1440,height:1000});
  await page.waitForURL(base+'/einrichtung');
  return {libraryIcons:true,straightActiveEdge:true,mainLinks:true,parentHierarchy:true,modalAlignment:true,pageActionAlignment:true,updatesInAbout:true,supportPreviewAndDownloads:true,desktopHomeRedirect:true,sectionClicks:true,selection:true,keyboard:true,cancelNoWrite:true,scopedSave:true,innerControls:true,activeRows:true,footerColumns:true,icons:true,detailRoutes:true,supportDraft:true,bundleRetryAndDownload:true,about:true,mobileWidths:[390,320],screenshots:out};
}
