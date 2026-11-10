import{h,add,api}from"./ui.js";
let auth,ui={},onPerf=null,created=null;
export function mountRunAds(root,a,perf){
  auth=a;onPerf=perf;root.innerHTML="";
  const w=h("div","wide");root.appendChild(w);
  w.appendChild(h("p","bz-note","Real money: every campaign is created PAUSED. Nothing spends until you press Activate. A safety cap limits the daily budget. Test with a small budget first."));
  const lab=(t,el,cls)=>{const l=h("label",cls||null,t);l.appendChild(el);return l};
  const inp=(ph,t)=>{const i=h("input");if(ph)i.placeholder=ph;if(t)i.type=t;return i};
  ui.acct=h("select");ui.name=inp("Campaign name");ui.obj=h("select");for(const[v,l]of[["traffic","Traffic: clicks to my website"],["awareness","Awareness: reach as many people as possible"]]){const o=h("option",null,l);o.value=v;ui.obj.appendChild(o)}
  ui.link=inp("https://yourwebsite.com/landing-page");ui.head=inp("Headline (max 40 characters)");ui.head.maxLength=40;ui.prim=h("textarea");ui.prim.rows=3;ui.prim.maxLength=125;ui.prim.placeholder="Primary text (max 125 characters)";
  ui.desc=inp("Description (max 30)");ui.desc.maxLength=30;ui.cta=h("select");for(const v of["LEARN_MORE","SHOP_NOW","SIGN_UP","CONTACT_US","BOOK_NOW","GET_QUOTE"]){const o=h("option",null,v.replace("_"," "));o.value=v;ui.cta.appendChild(o)}
  ui.img=inp("","file");ui.img.accept="image/jpeg,image/png";ui.countries=inp("Countries, e.g. ET, AE, QA");ui.ageMin=inp("18","number");ui.ageMax=inp("65","number");
  ui.gender=h("select");for(const[v,l]of[["all","All genders"],["female","Women"],["male","Men"]]){const o=h("option",null,l);o.value=v;ui.gender.appendChild(o)}
  ui.budget=inp("Daily budget","number");ui.days=inp("Days (default 7)","number");ui.topic=inp("What are you advertising? (for AI copy)");
  ui.ai=h("button","bz-btn ghost","Write headline and text with AI");ui.go=h("button","bz-btn","Create campaign (paused)");ui.msg=h("p","bz-mut");ui.out=h("div");
  ui.ai.onclick=async()=>{if(!ui.topic.value.trim()){ui.msg.textContent="Type what you are advertising first.";return}
    ui.ai.disabled=true;ui.msg.textContent="Writing…";try{const r=await api("POST","/api/optimize",{platform:"instagram",topic:ui.topic.value});const o=r.result||{};ui.head.value=((o.titles||[])[0]||"").slice(0,40);ui.prim.value=(o.hooks&&o.hooks[0]||o.description||"").slice(0,125);ui.msg.textContent="Review and edit the text."}catch(e){ui.msg.textContent=e.message}ui.ai.disabled=false};
  ui.go.onclick=create;
  const f=h("div","form");add(f,lab("Ad account",ui.acct),lab("Campaign name",ui.name),lab("Goal",ui.obj),lab("Website address (https)",ui.link,"wide"),lab("AI topic",ui.topic,"wide"),lab("Headline",ui.head),lab("Description",ui.desc),lab("Button",ui.cta),lab("Primary text",ui.prim,"wide"),lab("Image (JPG or PNG)",ui.img,"wide"),
    lab("Countries",ui.countries),lab("Age from",ui.ageMin),lab("Age to",ui.ageMax),lab("Who",ui.gender),lab("Daily budget",ui.budget),lab("Duration in days",ui.days));
  add(w,f,ui.ai,h("span",null," "),ui.go,ui.msg,ui.out);
  return refresh;
}
export async function refresh(){
  const d=window.__bzAdDraft;
  if(d){window.__bzAdDraft=null;ui.head.value=d.head||"";ui.prim.value=d.prim||"";ui.desc.value=d.desc||"";ui.topic.value=d.topic||"";if(d.cta)ui.cta.value=d.cta;ui.msg.textContent="Ad copy added from Ad creative. Add the link, image, audience and budget."}
  try{const d=await api("GET","/api/ads/accounts");ui.acct.innerHTML="";for(const a of d.accounts){const o=h("option",null,a.name+" ("+a.currency+")");o.value=a.id;ui.acct.appendChild(o)}
    ui.budget.placeholder="Daily budget (max "+d.maxDaily+")";ui.msg.textContent=""}
  catch(e){ui.acct.innerHTML="";ui.msg.textContent=e.message}
}
async function create(){
  if(!ui.img.files[0]){ui.msg.textContent="Choose an image for the ad.";return}
  const fd=new FormData();
  for(const[k,v]of[["account",ui.acct.value],["name",ui.name.value],["objective",ui.obj.value],["link",ui.link.value],["headline",ui.head.value],["primary",ui.prim.value],["description",ui.desc.value],["cta",ui.cta.value],["countries",ui.countries.value],["ageMin",ui.ageMin.value],["ageMax",ui.ageMax.value],["gender",ui.gender.value],["dailyBudget",ui.budget.value],["days",ui.days.value]])fd.append(k,v);
  fd.append("image",ui.img.files[0]);
  ui.go.disabled=true;ui.msg.textContent="Creating the campaign on Meta…";ui.out.innerHTML="";
  try{
    const tok=await auth.currentUser.getIdToken();
    const r=await fetch("/api/ads/launch",{method:"POST",headers:{Authorization:"Bearer "+tok},body:fd});
    if(!r.ok)throw new Error(await r.text());
    created=await r.json();ui.msg.textContent="Created. Review it, then activate.";drawCreated();
  }catch(e){ui.msg.textContent=e.message}
  ui.go.disabled=false;
}
function drawCreated(){
  const c=h("div","bz-card");c.style.marginTop="16px";c.appendChild(h("h3",null,"Campaign created (PAUSED)"));c.appendChild(h("p",null,created.summary));
  const l=h("a",null,"Review it in Meta Ads Manager");l.href="https://business.facebook.com/adsmanager/manage/campaigns?act="+created.account;l.target="_blank";l.rel="noopener";c.appendChild(l);
  c.appendChild(h("p","bz-mut","To start spending, type the daily budget again to confirm:"));
  const conf=h("input");conf.type="number";conf.placeholder="Type "+created.dailyBudget;
  const act=h("button","bz-btn","Activate and start spending"),st=h("p","bz-mut"),perf=h("button","bz-btn ghost","Analyze performance");perf.style.marginLeft="8px";
  act.onclick=async()=>{if(parseFloat(conf.value)!==created.dailyBudget){st.textContent="The amount does not match. Type "+created.dailyBudget+" to confirm.";return}
    act.disabled=true;st.textContent="Activating…";try{await api("POST","/api/ads/action",{type:"activate",id:created.campaignId,account:created.account});st.textContent="Active. Ads usually start after Meta's review (minutes to a few hours)."}catch(e){st.textContent=e.message}act.disabled=false};
  perf.onclick=()=>onPerf&&onPerf(created.account);
  add(c,conf,act,perf,st);ui.out.innerHTML="";ui.out.appendChild(c);
}
export function analyzeButton(){const b=h("button","bz-btn ghost","Analyze my Meta ads performance");b.onclick=()=>onPerf&&onPerf(ui.acct.value);return b}

// Campaigns managed report: ROAS, CPA, conversion rate and winning ads for the chosen period.
export function campaignsControls(onCamp){
  const w=h("div","apply-row");w.style.marginTop="12px";
  const sel=h("select");for(const[v,l]of[["last_30d","Last 30 days"],["last_90d","Last 90 days"],["this_year","This year"],["last_year","Last year"],["maximum","All time"]]){const o=h("option",null,l);o.value=v;sel.appendChild(o)}
  const b=h("button","bz-btn","Campaigns managed report (ROAS, CPA, winning ads)");b.style.marginLeft="8px";
  b.onclick=()=>onCamp&&onCamp(ui.acct.value,sel.value);
  add(w,sel,b);return w;
}
