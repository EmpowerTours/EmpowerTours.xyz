
const API='https://api.empowertours.xyz';
const EMBED=new Set(['hunt-dubai','faez','relocatedubai','sawweeli','cosecha','majlis','haqiqi','art','hunt','clearwave','backlog','turbo']); // apps whose frame-ancestors allow empowertours.xyz; each host must also be in the Caddyfile frame-src
const APPS=[
 {k:"hunt-dubai",n:"Hunt Dubai",r:"dubai",c:"#FF5CA8",h:"hunt-dubai.empowertours.xyz",d:"Short trails through Dubai's independent shops. Shops pay only for customers who really walked in.",m:["Go","AI agent","EN · AR · ES"],w:"no wallet needed",src:null,p:[-.55,-.35],how:["Pick a short trail through independent shops.", "Check in at each one: the counter QR indoors, GPS outside.", "The shop pays only for visits that were really verified."],f:["Bowlby One", "Bowlby+One", "400"]},
 {k:"faez",n:"Faez",r:"dubai",c:"#F28CA2",h:"faez.empowertours.xyz",d:"Reads every open Dubai eSupply tender against your trade licence and drafts the bid pack. Nothing is submitted until you approve it.",m:["Go","AI agent","tenders"],w:"sign-in opens full screen",src:null,p:[-.3,-.62],how:["Enter your trade licence.", "The agent reads every open eSupply tender and says why you can or can't bid.", "It drafts the bid pack; code checks every requirement is covered before you approve."],f:["Schibsted Grotesk", "Schibsted+Grotesk:wght@700", "700"]},
 {k:"relocatedubai",n:"RelocateDubai",r:"dubai",c:"#E9ECF5",h:"relocatedubai.empowertours.xyz",d:"From \"should we move?\" to settled in Dubai: visa routes checked against official sources, a cited budget, every step drafted for your approval.",m:["Go","AI agent","relocation"],w:"sign-in opens full screen",src:null,p:[-.72,.05],how:["Tell it about your family and income.", "Visa routes are checked against official sources; the budget cites each fee.", "Every payment and submission is drafted for your approval; you file it yourself."],f:["Overpass", "Overpass:wght@900", "900"]},
 {k:"sawweeli",n:"Sawweeli",r:"dubai",c:"#FF6B86",h:"sawweeli.empowertours.xyz",d:"Tell it your plan and it builds the trip inside your legal stay: real hotel rates, sample flights, one approval.",m:["Go","AI agent","travel"],w:"booking opens full screen",src:null,p:[-.42,.3],how:["Describe your plan in your own words.", "It works out your legal stay for your nationality, then finds real hotel rates.", "You approve once; the stay is booked inside your legal limit."],f:["Sofia Sans Extra Condensed", "Sofia+Sans+Extra+Condensed:wght@800", "800"]},
 {k:"cosecha",n:"Cosecha",r:"dubai",c:"#F5C431",h:"cosecha.empowertours.xyz",d:"What from your ranch can be sold in the Emirates? Mexican and UAE export rules checked and cited, papers drafted for you.",m:["Go","AI agent","ES · EN · AR"],w:"no wallet needed",src:null,p:[-.15,-.25],how:["Tell it what you produce, or send a photo.", "Mexican and UAE rules are checked, each with its official source.", "It drafts your paperwork; nothing is sent or paid without your approval."],f:["Anybody", "Anybody:wght@800", "800"]},
 {k:"majlis",n:"Asas",r:"dubai",c:"#6FA0FF",h:"asas.empowertours.xyz",d:"Plan a company setup in Dubai: reads the official licensing pages, checks every fee and ranks the options against your goal.",m:["Go","AI agent","company setup"],w:"no wallet needed",src:null,p:[-.62,.48],how:["Type your goal and budget.", "The agent reads official licensing pages; code checks every fee and link.", "Options are ranked by fit; you file and pay on each authority's own site."],f:["Readex Pro", "Readex+Pro:wght@600", "600"]},
 {k:"haqiqi",n:"Haqiqi",r:"dubai",c:"#B497FF",h:"haqiqi.empowertours.xyz",d:"Got a job or visa offer? Send a photo of the letter and check it against official UAE sources before you pay anyone.",m:["Go","AI agent","fraud check"],w:"no wallet needed",src:null,p:[-.25,.62],how:["Send a photo of the offer letter or a voice note.", "Names and numbers are removed, then claims are checked against official UAE sources.", "You see the warning signs and how to verify each one yourself."],f:["Figtree", "Figtree:wght@800", "800"]},
 {k:"art",n:"Art & Music",r:"monad",c:"#E8B04B",h:"art.empowertours.xyz",d:"Artists mint music and art on Monad and keep 90% of every licence sale. Minting is free: the platform pays the gas.",m:["TypeScript","Monad","Farcaster"],w:"passkey wallet: full screen",src:"fcempowertours",p:[.12,-.15],how:["Artists upload a track or artwork and mint it; we pay the gas.", "Collectors buy a licence; 90% goes to the artist on-chain.", "Listen on the radio; every play is counted."]},
 {k:"hunt",n:"Hunt",r:"monad",c:"#4FF0B4",h:"hunt.empowertours.xyz",d:"A GPS treasure hunt. Caches hide in the real world and your phone only says warmer or colder. Rewards pay real MON.",m:["TypeScript","Monad","passkeys"],w:"passkey wallet: full screen",src:"empowertours-hunt",p:[.45,-.4],how:["Make a wallet with your face or fingerprint.", "Walk: your phone only says warmer or colder.", "Claim the cache; the MON lands in your wallet."]},
 {k:"clearwave",n:"Clearwave",r:"monad",c:"#5FD3E3",h:"clearwave-production.up.railway.app",d:"Music royalty shares where the token contract itself refuses unverified buyers. Running on Monad testnet.",m:["Solidity","testnet","compliance"],w:"wallet: full screen",src:"clearwave",p:[.62,.05],how:["An artist offers a slice of future royalties.", "Investors verify once with an A-Pass.", "The token refuses unverified buyers on-chain; holders claim royalties."]},
 {k:"backlog",n:"Backlog",r:"monad",c:"#9EE493",h:"backlog-production-d1b6.up.railway.app",d:"An on-chain ledger of everything you build. Connect GitHub and an AI scores how finished each repo really is.",m:["Solidity","GitHub","AI"],w:"wallet: full screen",src:"backlog",p:[.35,.4],how:["Connect GitHub.", "An AI scores each repo against its own roadmap.", "Sign once and your build ledger lives on Monad."],f:["Archivo", "Archivo:wght@800", "800"]},
 {k:"turbo",n:"TURBO",r:"monad",c:"#C08BFF",h:"turbo-empowertours-production.up.railway.app",d:"A builder cohort for Latin America. Train, build, fund: weekly homework, governance and payouts.",m:["TypeScript","LATAM","cohort"],w:"Privy sign-in: full screen",src:"turbo-empowertours",p:[.7,.55],how:["Join a cohort.", "Ship weekly homework from a GitHub template.", "Vote, build and pitch for funding."],f:["Syne", "Syne:wght@800", "800"]},
];
const CODE=[["site-tape","Site Tape","Swift · ARKit"],["codi-direct","CoDi Direct","TypeScript · payments"],["rumbo","Rumbo","Solidity"],["pq-agent-batch","PQ Agent Batch","Solidity · SP1"],["quantum-portfolio","Quantum Portfolio","Python"]];
const RM=matchMedia('(prefers-reduced-motion: reduce)').matches;
const root=document.documentElement;
let sel=0;

/* terrain */
function H(x,z){
  const pk=(cx,cz,h,s)=>h*Math.exp(-((x-cx)**2+(z-cz)**2)/s);
  let y=pk(0,-.05,1,.07)+pk(-.42,.12,.7,.09)+pk(.44,.18,.62,.08)+pk(-.1,.5,.35,.12)+pk(.2,-.55,.3,.1);
  y*=1+.14*Math.sin(x*11+z*3)*Math.cos(z*9-x*2);
  return y;
}
function hex2rgb(h){const n=parseInt(h.slice(1),16);return[n>>16&255,n>>8&255,n&255]}
function mix(a,b,t){return a.map((v,i)=>Math.round(v+(b[i]-v)*t))}

function makeRenderer(cv,opts){
  const x=cv.getContext('2d');let W,Hh,dpr;
  function size(){dpr=Math.min(devicePixelRatio||1,2);W=cv.clientWidth;Hh=cv.clientHeight;cv.width=W*dpr;cv.height=Hh*dpr;x.setTransform(dpr,0,0,dpr,0,0)}
  size();new ResizeObserver(size).observe(cv);
  const N=34;
  function proj(px,py,pz,yaw,lift){
    const cy=Math.cos(yaw),sy=Math.sin(yaw);
    const xr=px*cy-pz*sy,zr=px*sy+pz*cy;
    const pitch=.34,cp=Math.cos(pitch),sp=Math.sin(pitch);
    const yy=py*lift*1.75-.35, y2=yy*cp-zr*sp, z2=yy*sp+zr*cp;
    const d=2.4, s=Math.min(W*1.05,Hh*1.6)*.6/(z2+d)*(opts.zoom||1);
    return [W/2+xr*s, Hh*(opts.base||.62)-y2*s, z2];
  }
  function draw(st){
    x.clearRect(0,0,W,Hh);
    const col=hex2rgb(st.color), dim=[38,52,111];
    const rows=Math.floor(N*st.reveal);
    x.lineWidth=1;
    for(let pass=0;pass<2;pass++){
      for(let i=0;i<=N;i++){
        if(pass===0 && i>rows) break;
        x.beginPath();let first=true,zs=0;
        for(let j=0;j<=N;j++){
          const u=-1+2*(pass?i:j)/N, v=-1+2*(pass?j:i)/N;
          if(pass===1 && j>rows) break;
          const p=proj(u,H(u,v),v,st.yaw,st.lift);zs+=p[2];
          first?x.moveTo(p[0],p[1]):x.lineTo(p[0],p[1]);first=false;
        }
        const depth=Math.max(0,Math.min(1,(zs/(N+1)+.9)/1.8));
        const c=mix(col,dim,depth*.85);
        x.strokeStyle=`rgba(${c[0]},${c[1]},${c[2]},${.85-depth*.6})`;x.stroke();
      }
    }
    const pins=[];
    if(st.pins>0){
      APPS.forEach((a,i)=>{
        if(i>=st.pins)return;
        const [px,pz]=a.p,py=H(px,pz);
        const p=proj(px,py,pz,st.yaw,st.lift);pins.push(p);
        const on=i===st.sel, c=a.c;
        if(on){
          const t=performance.now()/600, r=10+Math.sin(t)*3;
          x.strokeStyle=c;x.globalAlpha=.5;x.beginPath();x.arc(p[0],p[1],r+6,0,7);x.stroke();x.globalAlpha=1;
          x.beginPath();x.moveTo(p[0],p[1]);x.lineTo(p[0],p[1]-46);x.strokeStyle=c;x.stroke();
          x.font='600 12px "Geist Mono",monospace';const tw=x.measureText(a.n.toUpperCase()).width+16;
          x.fillStyle='#050816';x.fillRect(p[0]-tw/2,p[1]-70,tw,22);x.strokeRect(p[0]-tw/2,p[1]-70,tw,22);
          x.fillStyle=c;x.textAlign='center';x.fillText(a.n.toUpperCase(),p[0],p[1]-55);
        } else if(i===st.hover){
          x.font='500 11px "Geist Mono",monospace';x.fillStyle='#E8ECFA';x.textAlign='center';x.fillText(a.n,p[0],p[1]-14);
        }
        x.fillStyle=c;x.globalAlpha=.22;x.beginPath();x.arc(p[0],p[1],on?14:9,0,7);x.fill();x.globalAlpha=1;x.beginPath();x.arc(p[0],p[1],on?6:4.5,0,7);x.fill();
      });
    }
    return pins;
  }
  return {draw,size};
}


/* ---------- production wiring ---------- */
let CONTRIB=0;
function seenIntro(){try{return localStorage.getItem('et_intro')==='1'}catch{return false}}
function markIntro(){try{localStorage.setItem('et_intro','1')}catch{}}

function showApp(a){
  const vp=document.querySelector('.viewport');
  vp.querySelector('iframe')?.remove();vp.querySelector('.try')?.remove();
  const img=document.getElementById('dimg');img.hidden=false;img.src=`/shots/${a.k}.webp`;img.alt=`${a.n} running live`;
  const note=vp.querySelector('.note span');
  if(EMBED.has(a.k)){
    note.innerHTML='<b>Live app.</b> Browse it right here; wallet steps open full screen.';
    const b=document.createElement('button');b.type='button';b.className='btn primary try';b.textContent='▶ Try it live here';
    b.addEventListener('click',()=>{
      const f=document.createElement('iframe');
      f.src='https://'+a.h;f.title=a.n+' (live app)';f.loading='lazy';f.referrerPolicy='strict-origin-when-cross-origin';
      f.setAttribute('sandbox','allow-scripts allow-same-origin allow-forms allow-popups allow-popups-to-escape-sandbox');
      f.setAttribute('allow','clipboard-write');
      img.hidden=true;b.remove();vp.prepend(f);
    });
    vp.appendChild(b);
  } else {
    note.innerHTML='<b>Opens full screen.</b> This app runs on its own site.';
  }
}

async function loadGitHub(){
  const cal=document.getElementById('cal'),list=document.querySelector('#log .commits');
  try{
    const r=await fetch(API+'/api/v1/site/github',{mode:'cors'});
    if(!r.ok)throw 0;
    const {data}=await r.json();
    CONTRIB=data.total||0;
    document.querySelectorAll('[data-contrib]').forEach(e=>e.textContent=CONTRIB.toLocaleString('en-US'));
    document.querySelectorAll('#log .panel h3 small').forEach((e,i)=>{if(i===0)e.textContent=CONTRIB.toLocaleString('en-US')+' in the last year'});
    if(cal&&Array.isArray(data.days)&&data.days.length){
      cal.textContent='';
      const first=new Date(data.days[0].date+'T00:00:00Z').getUTCDay();
      for(let i=0;i<first;i++){const x=document.createElement('i');x.style.visibility='hidden';cal.appendChild(x)}
      for(const d of data.days){const x=document.createElement('i');x.dataset.l=String(Math.max(0,Math.min(4,d.level|0)));x.title=`${d.count|0} contributions on ${String(d.date).slice(0,10)}`;cal.appendChild(x)}
      cal.parentElement.scrollLeft=cal.parentElement.scrollWidth;
    }
    if(list&&Array.isArray(data.commits)){
      list.textContent='';
      for(const c of data.commits){
        const li=document.createElement('li');
        const code=document.createElement('code');code.textContent=String(c.sha||'').slice(0,7);
        const m=document.createElement('a');m.className='m';m.textContent=String(c.message||'');
        if(/^https:\/\/github\.com\//.test(c.url||'')){m.href=c.url;m.rel='noopener noreferrer';m.target='_blank'}
        const rr=document.createElement('span');rr.className='r';
        const dt=new Date(c.at);rr.textContent=`${String(c.repo||'')} · ${isNaN(dt)?'':dt.toLocaleDateString('en-US',{month:'short',day:'2-digit'})}`;
        li.append(code,m,rr);list.appendChild(li);
      }
    }
  }catch{
    if(list){list.textContent='';const li=document.createElement('li');const a=document.createElement('a');a.href='https://github.com/EmpowerTours';a.rel='noopener noreferrer';a.textContent='See our latest commits on GitHub →';li.appendChild(a);list.appendChild(li)}
  }
}

function say(id,msg,ok){const p=document.getElementById(id);p.hidden=false;p.className='formmsg '+(ok?'ok':'err');p.textContent=msg}

function wireForms(){
  const inq=document.getElementById('inq');
  inq.addEventListener('submit',async e=>{
    e.preventDefault();
    const fd=new FormData(inq),btn=inq.querySelector('button[type=submit]');
    const body={name:fd.get('name'),email:fd.get('email'),type:fd.get('type'),budget:fd.get('budget'),when:fd.get('when'),repo:fd.get('repo'),message:fd.get('message'),website:fd.get('website')||'',source:inq.dataset.source||'form'};
    if(!String(body.name||'').trim()||!String(body.email||'').trim()||!String(body.message||'').trim())return say('inq-msg','Add your name, email and what you want built.');
    btn.disabled=true;btn.textContent='Sending…';
    try{
      const r=await fetch(API+'/api/v1/site/inquiry',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(body)});
      const j=await r.json().catch(()=>({}));
      if(!r.ok)throw new Error(j.error||'Your inquiry didn\'t send. Try again.');
      inq.reset();say('inq-msg','Thanks. Your inquiry is in; we\'ll reply by email within two business days.',true);
    }catch(err){say('inq-msg',err.message||'Your inquiry didn\'t send. Try again.')}
    finally{btn.disabled=false;btn.textContent='Send inquiry'}
  });

  // Careers: encrypted in the browser (same scheme as csecsystems.com).
  // AES-256-GCM over a JSON bundle; the AES key is wrapped with RSA-OAEP-SHA256 (public key /apply-key.json).
  const job=document.getElementById('job');
  const b64=buf=>{const u=new Uint8Array(buf);let s='';for(let i=0;i<u.length;i+=0x8000)s+=String.fromCharCode(...u.subarray(i,i+0x8000));return btoa(s)};
  const fromB64=s=>Uint8Array.from(atob(s),c=>c.charCodeAt(0));
  job.addEventListener('submit',async e=>{
    e.preventDefault();
    const fd=new FormData(job),btn=job.querySelector('button[type=submit]'),file=fd.get('cv');
    if(!String(fd.get('name')||'').trim()||!String(fd.get('email')||'').trim())return say('job-msg','Add your name and email.');
    if(file&&file.size){if(file.size>5*1024*1024)return say('job-msg','Your CV is over 5 MB. Send a smaller PDF.');if(!/\.pdf$/i.test(file.name))return say('job-msg','Send your CV as a PDF.')}
    if(!fd.get('consent'))return say('job-msg','Tick the box to let us keep your application.');
    if(!window.crypto?.subtle)return say('job-msg','Your browser can\'t encrypt files. Update it and try again.');
    btn.disabled=true;btn.textContent='Encrypting…';
    try{
      const k=await fetch('/apply-key.json',{cache:'no-store'}).then(r=>{if(!r.ok)throw new Error('Applications aren\'t open right now.');return r.json()});
      const rsa=await crypto.subtle.importKey('spki',fromB64(k.spki),{name:'RSA-OAEP',hash:'SHA-256'},false,['encrypt']);
      const fields={};for(const n of ['name','email','github','location','shipped','message'])fields[n]=String(fd.get(n)||'').trim();
      const bundle={fields,skills:fd.getAll('skill'),file:file&&file.size?{name:file.name,type:file.type,b64:b64(await file.arrayBuffer())}:null,sentAt:new Date().toISOString()};
      const aes=await crypto.subtle.generateKey({name:'AES-GCM',length:256},true,['encrypt']);
      const iv=crypto.getRandomValues(new Uint8Array(12));
      const ct=await crypto.subtle.encrypt({name:'AES-GCM',iv},aes,new TextEncoder().encode(JSON.stringify(bundle)));
      const ek=await crypto.subtle.encrypt({name:'RSA-OAEP'},rsa,await crypto.subtle.exportKey('raw',aes));
      btn.textContent='Sending…';
      const r=await fetch(API+'/api/v1/site/application',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({v:1,kid:k.kid,ek:b64(ek),iv:b64(iv),ct:b64(ct),website:fd.get('website')||''})});
      const j=await r.json().catch(()=>({}));
      if(!r.ok)throw new Error(j.error||'Your application didn\'t send. Try again.');
      job.reset();say('job-msg','Sent securely. Your application was encrypted on your device, and only EmpowerTours can open it.',true);
    }catch(err){say('job-msg',err.message||'Your application didn\'t send. Try again.')}
    finally{btn.disabled=false;btn.textContent='Encrypt and send'}
  });
}

/* main mountain */
const mcv=document.getElementById('mtnCv');
const R=makeRenderer(mcv,{zoom:1.05,base:innerWidth<980?.8:.68});
const ST={yaw:0,tYaw:0,lift:1,reveal:1,pins:12,sel:0,hover:-1,color:APPS[0].c,cur:hex2rgb(APPS[0].c)};
let PINS=[],visible=true,introOn=false,last=0;
function targetYaw(i){const [px,pz]=APPS[i].p;let t=-Math.PI/2-Math.atan2(pz,px);let d=t-ST.yaw;d=Math.atan2(Math.sin(d),Math.cos(d));return ST.yaw+d*.55}
function frame(now){
  if(visible&&!introOn&&now-last>32){last=now;
    ST.yaw+= (ST.tYaw-ST.yaw)*.06 + (RM?0:.0006);
    ST.tYaw+= RM?0:.0006;
    const tc=hex2rgb(APPS[ST.sel].c);ST.cur=ST.cur.map((v,i)=>v+(tc[i]-v)*.08);
    ST.color='#'+ST.cur.map(v=>Math.round(v).toString(16).padStart(2,'0')).join('');
    PINS=R.draw(ST);
  }
  requestAnimationFrame(frame);
}
new IntersectionObserver(e=>visible=e[0].isIntersecting).observe(mcv);
function pick(ev){const r=mcv.getBoundingClientRect(),mx=ev.clientX-r.left,my=ev.clientY-r.top;let b=-1,bd=24;PINS.forEach((p,i)=>{const d=Math.hypot(p[0]-mx,p[1]-my);if(d<bd){bd=d;b=i}});return b}
mcv.addEventListener('mousemove',e=>{ST.hover=pick(e);mcv.style.cursor=ST.hover>=0?'pointer':'crosshair'});
mcv.addEventListener('click',e=>{const i=pick(e);if(i>=0)select(i,e.clientX,e.clientY)});

function select(i,fx,fy){
  sel=i;ST.sel=i;ST.tYaw=targetYaw(i);const a=APPS[i];
  root.style.setProperty('--acc',a.c);setFont(a);
  if(fx===undefined){const r=mcv.getBoundingClientRect();const p=PINS[i]||[r.width/2,r.height/2];fx=r.left+p[0];fy=r.top+p[1]}
  const f=document.getElementById('flood');if(!RM){f.style.setProperty('--fx',fx+'px');f.style.setProperty('--fy',fy+'px');f.classList.remove('go');void f.offsetWidth;f.classList.add('go')}
  document.getElementById('cName').textContent=a.n;
  document.getElementById('cRoute').textContent=a.r==='dubai'?'Dubai route · AI Agents Championship':'Monad range · on-chain';
  document.getElementById('cDesc').textContent=a.d;
  document.getElementById('cHow').innerHTML=a.how.map(s=>`<li>${s}</li>`).join('');
  document.getElementById('durl').textContent=a.h;
  document.getElementById('dopen').href=document.getElementById('cOpen').href='https://'+a.h;
  showApp(a);
  document.getElementById('dwallet').textContent=a.w;
  const s=document.getElementById('cSrc');
  if(a.src){s.href='https://github.com/EmpowerTours/'+a.src;s.textContent='Source ↗';s.removeAttribute('aria-disabled')}else{s.removeAttribute('href');s.textContent='Source: private repo · championship entry';s.setAttribute('aria-disabled','true')}
  document.getElementById('cMeta').innerHTML='<span class="chip live">● live</span>'+a.m.map(m=>`<span class="chip">${m}</span>`).join('');
  document.getElementById('hudN').textContent=String(i+1).padStart(2,'0');
  document.getElementById('hudR').textContent=a.r==='dubai'?'DUBAI ROUTE':'MONAD RANGE';
  document.querySelectorAll('#chips2 button').forEach((b,j)=>b.setAttribute('aria-pressed',j===i));
}
document.getElementById('chips2').innerHTML=APPS.map((a,i)=>`<button type="button" data-i="${i}" style="--c:${a.c}">${a.n}</button>`).join('');
document.getElementById('chips2').addEventListener('click',e=>{const b=e.target.closest('button');if(b)select(+b.dataset.i)});


/* per-app display font, loaded on demand */
const loaded=new Set();
function setFont(a){
  if(a.f){
    if(!loaded.has(a.f[1])){loaded.add(a.f[1]);const l=document.createElement('link');l.rel='stylesheet';l.href=`https://fonts.googleapis.com/css2?family=${a.f[1]}&display=swap`;document.head.appendChild(l);
      l.onload=()=>document.fonts.load(`${a.f[2]} 40px "${a.f[0]}"`).then(fitAll).catch(()=>{})}
    else document.fonts.load(`${a.f[2]} 40px "${a.f[0]}"`).then(fitAll).catch(()=>{});
    root.style.setProperty('--f-display',`"${a.f[0]}","Big Shoulders Display",sans-serif`);root.style.setProperty('--d-weight',a.f[2]);root.style.setProperty('--d-case','none');root.style.setProperty('--d-track','-.02em');
  } else {['--f-display','--d-weight','--d-case','--d-track'].forEach(p=>root.style.removeProperty(p))}
  requestAnimationFrame(fitAll);
}
/* fit one-line wordmarks to their box so they never clip */
function fit(el,max){if(!el||!el.getClientRects().length)return;el.style.fontSize=max+'px';const pp=getComputedStyle(el.parentElement),ep=getComputedStyle(el);const w=el.parentElement.clientWidth-parseFloat(pp.paddingLeft)-parseFloat(pp.paddingRight)-parseFloat(ep.paddingLeft)-parseFloat(ep.paddingRight);const r=document.createRange();r.selectNodeContents(el);const s=r.getBoundingClientRect().width;if(s>w)el.style.fontSize=Math.floor(max*w/s*.98)+'px'}
function fitAll(){fit(document.querySelector('.wordmark'),220);fit(document.getElementById('introWord'),128)}
addEventListener('resize',fitAll);document.fonts.ready.then(fitAll);document.fonts.addEventListener('loadingdone',fitAll);

/* routes */
(function(){
  const g=(r,t,s)=>`<div class="route"><div><h3>${t}</h3><small>${s}</small></div><div class="trail">`+
    APPS.map((a,i)=>a.r===r?`<button class="wp" type="button" data-i="${i}" style="--c:${a.c}"><i></i><span>${a.n}</span><em>try it ↑</em></button>`:'').join('')+`</div></div>`;
  document.getElementById('routesEl').innerHTML=g('dubai','Dubai route','7 AI agents · championship 2026')+g('monad','Monad range','5 on-chain apps · chain 143')+
   `<div class="route"><div><h3>Field tools</h3><small>source only · public repos</small></div><div class="trail">`+CODE.map(([r,n,l])=>`<a class="wp code" href="https://github.com/EmpowerTours/${r}"><i></i><span>${n}</span><em>${l}</em></a>`).join('')+`</div></div>`;
  document.getElementById('routesEl').addEventListener('click',e=>{const b=e.target.closest('button.wp');if(!b)return;document.getElementById('drive').scrollIntoView({behavior:RM?'auto':'smooth'});setTimeout(()=>select(+b.dataset.i),RM?0:500)});
})();

/* keyboard + palette */
const pal=document.getElementById('pal'),palIn=document.getElementById('palIn'),palList=document.getElementById('palList');
const ITEMS=[...APPS.map((a,i)=>({t:a.n,s:a.r==='dubai'?'Dubai app':'Monad app',go:()=>{document.getElementById('drive').scrollIntoView();select(i)}})),
 ...[['Routes','routes'],['Shipping log','log'],['What we stand for','why'],['Hire us on your repo','hire'],['Start a project','contact'],['Careers','join']].map(([t,id])=>({t,s:'section',go:()=>document.getElementById(id).scrollIntoView()}))];
let pi=0,shown=ITEMS;
function renderPal(){const q=palIn.value.toLowerCase();shown=ITEMS.filter(it=>it.t.toLowerCase().includes(q));pi=Math.min(pi,Math.max(0,shown.length-1));
  palList.innerHTML=shown.map((it,i)=>`<li class="${i===pi?'sel':''}" data-i="${i}">${it.t}<small>${it.s}</small></li>`).join('')}
function openPal(){pal.classList.add('on');palIn.value='';pi=0;renderPal();palIn.focus()}
function closePal(){pal.classList.remove('on')}
document.getElementById('kbtn').addEventListener('click',openPal);
pal.addEventListener('click',e=>{if(e.target===pal)closePal();const li=e.target.closest('li');if(li){closePal();shown[+li.dataset.i].go()}});
palIn.addEventListener('input',()=>{pi=0;renderPal()});
palIn.addEventListener('keydown',e=>{if(e.key==='ArrowDown'){pi=Math.min(pi+1,shown.length-1);renderPal();e.preventDefault()}else if(e.key==='ArrowUp'){pi=Math.max(pi-1,0);renderPal();e.preventDefault()}else if(e.key==='Enter'&&shown[pi]){closePal();shown[pi].go()}else if(e.key==='Escape')closePal()});
addEventListener('keydown',e=>{
  if((e.metaKey||e.ctrlKey)&&e.key.toLowerCase()==='k'){e.preventDefault();pal.classList.contains('on')?closePal():openPal();return}
  if(/INPUT|TEXTAREA|SELECT/.test(document.activeElement.tagName))return;
  if(e.key==='ArrowRight')select((sel+1)%APPS.length);
  if(e.key==='ArrowLeft')select((sel+APPS.length-1)%APPS.length);
});


/* intro */
function runIntro(){
  const el=document.getElementById('intro');el.classList.remove('done');el.style.display='grid';introOn=true;fitAll();
  if(RM){finish();return}
  const IR=makeRenderer(document.getElementById('introCv'),{zoom:1.3,base:.92});
  const word=document.getElementById('introWord'),alt=document.getElementById('alti');word.classList.remove('on');
  const total=CONTRIB||3400,t0=performance.now();let stop=false;
  function step(now){
    if(stop)return;const t=(now-t0)/1000;
    const reveal=Math.min(1,t/1.4), lift=t<1.2?0:Math.min(1,(t-1.2)/1.6), e=1-Math.pow(1-lift,3);
    const pins=t<3.2?0:Math.min(12,Math.floor((t-3.2)/.09)+1);
    IR.draw({yaw:-.4+t*.12,lift:e,reveal,pins,sel:-1,hover:-1,color:'#A9C4FF'});
    alt.textContent=Math.round(total*Math.min(1,t/2.8)).toLocaleString('en-US');
    if(t>2.6&&!word.classList.contains('on')){fitAll();word.classList.add('on')}
    if(t>5)return finish();
    requestAnimationFrame(step);
  }
  function finish(){stop=true;introOn=false;el.classList.add('done');setTimeout(()=>el.style.display='none',800)}
  document.getElementById('skip').onclick=finish;
  requestAnimationFrame(step);
}

(function(){
  const notes={pr:["Quote","Each task gets a fixed price before work starts. You pay only when you merge its pull request, so you never pay for time you can't check or work you didn't accept."],
               hr:["Hours","An hourly rate drawn from your balance. Simple to start, but nobody can really check hours spent on code, so we only offer it for open-ended support."]};
  document.querySelectorAll('.toggle button').forEach(b=>b.addEventListener('click',()=>{
    document.querySelectorAll('.toggle button').forEach(x=>x.setAttribute('aria-pressed',x===b));
    const n=notes[b.dataset.m];document.getElementById('qcol').textContent=n[0];document.getElementById('pnote').textContent=n[1];
    const rows=document.querySelectorAll('#lrows tr td:nth-child(3)');const v=b.dataset.m==='pr'?['120 MON','200 MON','150 MON']:['6 h · 120 MON','10 h · 200 MON','est. 7.5 h'];
    rows.forEach((td,i)=>td.textContent=v[i]);}));
})();
select(0);requestAnimationFrame(frame);
if(!seenIntro()){runIntro();markIntro()}else{document.getElementById('intro').style.display='none'}
loadGitHub();wireForms();
