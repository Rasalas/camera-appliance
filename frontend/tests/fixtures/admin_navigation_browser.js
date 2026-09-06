async page => {
  const base='http://127.0.0.1:18174', out='output/playwright';
  await page.unrouteAll({behavior:'ignoreErrors'});
  const settings={'viewer.performance.mode':'quality','auth.session_hours':'12','auth.viewer_public':'false','auth.local_admin_bypass':'false','network.lan_access_enabled':'true','camera.relay.ids':'test','camera.relay.test.name':'Test relay','camera.relay.test.ssh_target':'relay-host'};
  const auth={enabled:true,role:'admin',authenticated:true,admin_password_set:true,viewer_password_set:true,session_hours:12};
  const version={version:'0.5.0',commit:'63f2651',status:'ok'};
  const devices=[{id:'courtyard',manufacturer:'Test',model:'Hofkamera',last_ip:'192.0.2.10',raw_json:{rtsp_port_open:true}}];
  const identities=[{id:'shared',name:'Gemeinsamer Zugang',username:'viewer',password_set:true}];
  const events=[{id:'1',created_at:'2026-09-06T08:00:00Z',type:'camera.connected',level:'info',message:'Kameraverbindung wiederhergestellt.'}];
  let saveCount=0,failBundle=true,bundlePosts=0;
  await page.route(base+'/api/**',async route=>{
    const path=route.request().url().slice(base.length).split('?')[0],method=route.request().method();
    let data=[];
    if(path==='/api/auth/status')data=auth;
    else if(path==='/api/health')data=version;
    else if(path==='/api/settings'){if(method==='PUT'){Object.assign(settings,route.request().postDataJSON());saveCount++}data=settings}
    else if(path==='/api/status')data={devices,slots:[],bindings:[],relays:[],system:{camera_appliance:{online:true},go2rtc:{online:true}},watchdog:{enabled:false},version,recent_events:events,scan_runs:[]};
    else if(path==='/api/system/update/status')data={phase:'idle'};
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
  await page.getByText('Diagnoseauszug prüfen',{exact:true}).click();
  await page.getByLabel('Diagnoseauszug',{exact:true}).waitFor();
  await page.getByLabel('Was funktioniert nicht?').fill('Das Bild flackert & friert ein.');
  let href=await page.getByRole('link',{name:'Hilfe anfragen',exact:true}).getAttribute('href');
  if(!href.startsWith('mailto:mail@tbuck.de?') || decodeURIComponent(href).includes('Kameraverbindung'))throw Error('Mail recipient or diagnostics opt-in wrong');
  await page.getByLabel('Diagnoseauszug in die E-Mail aufnehmen').check();
  href=await page.getByRole('link',{name:'Hilfe anfragen',exact:true}).getAttribute('href');
  if(!decodeURIComponent(href).includes('Kameraverbindung wiederhergestellt.'))throw Error('Mail missing selected logs');
  await page.getByRole('button',{name:'Support-Bundle erstellen',exact:true}).click();
  await page.getByRole('alert').filter({hasText:'Test: Erstellung fehlgeschlagen'}).waitFor();
  if(await page.getByLabel('Was funktioniert nicht?').inputValue()!=='Das Bild flackert & friert ein.')throw Error('Bundle failure lost description');
  await page.getByRole('button',{name:'Support-Bundle erstellen',exact:true}).click();
  const download=page.waitForEvent('download');
  await page.getByRole('link',{name:'Bundle herunterladen',exact:true}).click();
  if(!(await download).suggestedFilename().endsWith('.tar.gz') || bundlePosts!==2)throw Error('Download failed');
  await page.screenshot({path:out+'/support-desktop.png',fullPage:true,animations:'disabled'});
  await page.getByLabel('Was funktioniert nicht?').fill('');
  await page.goto(base+'/system/ueber');
  await page.getByRole('link',{name:/Buy Me a Coffee/}).waitFor();
  await page.getByText('MIT-Lizenz lesen',{exact:true}).click();
  await page.getByText('Copyright (c) 2026 Torben Buck',{exact:false}).waitFor();
  await page.screenshot({path:out+'/about-desktop.png',fullPage:true,animations:'disabled'});
  for(const width of [390,320]){
    await page.setViewportSize({width,height:844});
    for(const path of ['/verwaltung','/system/zugriff','/system/wartung/support','/system/ueber']){
      await page.goto(base+path);await page.getByRole('heading',{level:1}).waitFor();
      if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error(`Overflow at ${width}: ${path}`);
      if(width===390 && path!=='/verwaltung')await page.screenshot({path:out+'/'+path.split('/').pop()+'-mobile.png',fullPage:true,animations:'disabled'});
    }
  }
  await page.goto(base+'/verwaltung');
  await page.getByRole('heading',{level:1,name:'Home'}).waitFor();
  await page.setViewportSize({width:1440,height:1000});
  await page.waitForURL(base+'/einrichtung');
  return {desktopHomeRedirect:true,sectionClicks:true,selection:true,keyboard:true,cancelNoWrite:true,scopedSave:true,innerControls:true,activeRows:true,footerColumns:true,icons:true,detailRoutes:true,supportDraft:true,bundleRetryAndDownload:true,about:true,mobileWidths:[390,320],screenshots:out};
}
