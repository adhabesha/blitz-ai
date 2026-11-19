/* ficx tracking: Meta Pixel, Google Analytics 4 and Google Tag Manager, loaded only after the visitor accepts cookies.
   IDs come from /api/public/config (set in Admin > Tracking). Other scripts call window.fxTrack("lead"|"sign_up"|"website_check", {...}). */
(()=>{
const API=(window.FICX_API||"").replace(/\/+$/,"");
const K="ficx.consent";
let cfg=null,on=false,queue=[];
const get=()=>{try{return localStorage.getItem(K)}catch(e){return null}};
const set=v=>{try{localStorage.setItem(K,v)}catch(e){}};
const MAP={lead:{ga:"generate_lead",fb:["track","Lead"],gtm:"ficx_lead"},sign_up:{ga:"sign_up",fb:["track","CompleteRegistration"],gtm:"ficx_sign_up"},website_check:{ga:"website_check",fb:["trackCustom","WebsiteCheck"],gtm:"ficx_website_check"}};
function send(name,p){
  const m=MAP[name];if(!m)return;p=p||{};
  try{if(window.dataLayer&&cfg.gtm)window.dataLayer.push(Object.assign({event:m.gtm},p))}catch(e){}
  try{if(window.gtag&&cfg.ga4)window.gtag("event",m.ga,p)}catch(e){}
  try{if(window.fbq&&cfg.pixel)window.fbq(m.fb[0],m.fb[1],p)}catch(e){}
}
window.fxTrack=(name,p)=>{if(on)send(name,p);else if(get()!=="no"&&queue.length<20)queue.push([name,p])};
function script(src){const s=document.createElement("script");s.async=true;s.src=src;document.head.appendChild(s)}
function load(){
  if(on||!cfg)return;on=true;
  window.dataLayer=window.dataLayer||[];
  if(cfg.gtm){window.dataLayer.push({"gtm.start":Date.now(),event:"gtm.js"});script("https://www.googletagmanager.com/gtm.js?id="+encodeURIComponent(cfg.gtm))}
  if(cfg.ga4){window.gtag=function(){window.dataLayer.push(arguments)};window.gtag("js",new Date());window.gtag("config",cfg.ga4);script("https://www.googletagmanager.com/gtag/js?id="+encodeURIComponent(cfg.ga4))}
  if(cfg.pixel){
    const f=window.fbq=function(){f.callMethod?f.callMethod.apply(f,arguments):f.queue.push(arguments)};
    if(!window._fbq)window._fbq=f;f.push=f;f.loaded=true;f.version="2.0";f.queue=[];
    script("https://connect.facebook.net/en_US/fbevents.js");window.fbq("init",cfg.pixel);window.fbq("track","PageView");
  }
  for(const[n,p]of queue)send(n,p);queue=[];
}
function banner(){
  const b=document.createElement("div");b.setAttribute("role","dialog");b.setAttribute("aria-label","Cookies");
  b.style.cssText="position:fixed;left:12px;right:12px;bottom:12px;z-index:9999;max-width:560px;margin:0 auto;background:#142748;color:#f2f8f8;border:1px solid rgba(255,255,255,.18);border-radius:14px;padding:14px 16px;font:14px/1.45 Inter,system-ui,sans-serif;box-shadow:0 10px 30px rgba(0,0,0,.35)";
  b.innerHTML='<div style="margin-bottom:10px">We use cookies to measure visits and the ads that bring people here. Nothing is loaded unless you accept.</div>';
  const row=document.createElement("div");row.style.cssText="display:flex;gap:8px;justify-content:flex-end";
  const mk=(t,fill,fn)=>{const x=document.createElement("button");x.textContent=t;x.style.cssText="font:inherit;font-weight:600;border-radius:999px;padding:8px 16px;cursor:pointer;border:1px solid "+(fill?"#da602c":"rgba(255,255,255,.35)")+";background:"+(fill?"#da602c":"transparent")+";color:#fff";x.onclick=()=>{fn();b.remove()};return x};
  row.appendChild(mk("No thanks",false,()=>{set("no");queue=[]}));row.appendChild(mk("Accept",true,()=>{set("yes");load()}));
  b.appendChild(row);document.body.appendChild(b);
}
fetch(API+"/api/public/config").then(r=>r.json()).then(c=>{
  cfg=c||{};if(!(cfg.pixel||cfg.ga4||cfg.gtm))return;
  const v=get();
  if(v==="yes")load();else if(v!=="no"){if(document.body)banner();else document.addEventListener("DOMContentLoaded",banner)}
}).catch(()=>{});
})();
