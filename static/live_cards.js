// Live cards for connected accounts: two per row, built from the latest saved report of each account.
// Nothing is re-analysed on open; "Refresh" re-reads the account. Fixed issues are marked Done and stay dismissed
// unless the next refresh still finds the problem.
import{h,add}from"./ui.js";
const A=x=>Array.isArray(x)?x:[];
const key=c=>(c.category||"")+"|"+c.name;
const ago=t=>{if(!t)return"never";const s=(Date.now()-new Date(t))/1000;return s<60?"just now":s<3600?Math.floor(s/60)+" min ago":s<86400?Math.floor(s/3600)+" h ago":new Date(t).toLocaleDateString()};
const GC={"g:yt":"YouTube","g:sc":"Search (Google)","g:ga":"Traffic (GA4)"};
const open=(r,id)=>A(r&&r.checks).filter(c=>c.status!=="pass"&&(!GC[id]||c.category===GC[id]));
const kpisFor=(r,id)=>{const k=A(r&&r.kpis);if(!GC[id])return k;const cls=l=>/search|position|click|impression|ctr/i.test(l)?"g:sc":/session|users|key event|bounce|conversion|engaged|traffic/i.test(l)?"g:ga":"g:yt";return k.filter(x=>cls(x.label||"")===id)};
function store(uid,id){const k="fx-fix:"+uid+":"+id;
  const get=()=>{try{return JSON.parse(localStorage.getItem(k))||{done:{},seen:[],reopened:[]}}catch(e){return{done:{},seen:[],reopened:[]}}};
  const put=v=>{try{localStorage.setItem(k,JSON.stringify(v))}catch(e){}};return{get,put}}
// called after every fresh report
export function reconcile(uid,id,r){const st=store(uid,id),v=st.get();const now=new Set(open(r,id).map(key));v.reopened=[];
  for(const k of Object.keys(v.done)){if(v.done[k].manual&&now.has(k)){delete v.done[k];v.reopened.push(k)}else if(!v.done[k].manual&&now.has(k)){delete v.done[k]}}
  for(const k of A(v.seen))if(!now.has(k)&&!v.done[k])v.done[k]={at:new Date().toISOString(),manual:false};
  v.seen=[...now];st.put(v);return v}
export function liveCards(box,o){
  // o: {uid, api, getHist, addHist, providers:[[id,name]], connected:Set, open:(r)=>void, useAI:()=>bool}
  const exp=new Set();let busy=new Set();
  const latest=id=>GC[id]?A(o.getHist()).filter(r=>r.kind==="google").sort((a,b)=>new Date(b.createdAt)-new Date(a.createdAt))[0]:A(o.getHist()).filter(r=>r.kind==="social_connected"&&((r.meta||{}).provider===id||(!(r.meta||{}).provider&&String(r.target||"").toLowerCase().startsWith(o.name(id).toLowerCase().split(" ")[0])))).sort((a,b)=>new Date(b.createdAt)-new Date(a.createdAt))[0];
  async function refresh(id){if(GC[id]){const g=Object.keys(GC);g.forEach(x=>busy.add(x));draw();try{const r=await o.googleRun();o.addHist(r);g.forEach(x=>reconcile(o.uid,x,r))}catch(e){o.say(e.message)}g.forEach(x=>busy.delete(x));draw();return}busy.add(id);draw();try{const r=await o.api("POST","/api/evaluate",{kind:"social_connected",query:id,useAI:false});o.addHist(r);reconcile(o.uid,id,r)}catch(e){o.say(e.message)}busy.delete(id);draw()}
  function draw(){box.innerHTML="";const ids=[...o.connected,...(o.google?Object.keys(GC):[])];
    const hd=h("div","lv-head"),t=h("div");add(t,h("h2",null,"Your connected accounts, live"),h("p","bz-mut","Saved analysis for each account. Press Refresh to read the latest numbers; nothing is re-analysed until you ask."));
    const all=h("button","bz-btn ghost","Refresh all");all.onclick=async()=>{let g=false;for(const id of ids){if(GC[id]){if(g)continue;g=true}await refresh(id)}};add(hd,t,ids.length?all:null);box.appendChild(hd);
    if(!ids.length){box.appendChild(h("p","bz-mut","Connect an account above and its live card appears here."));return}
    const g=h("div","lv-grid");box.appendChild(g);
    for(const id of ids){const r=latest(id),c=h("article","lv-card"+(exp.has(id)?" big":""));
      const v=store(o.uid,id).get(),nm=GC[id]?({"g:yt":"YouTube channel","g:sc":"Google Search Console","g:ga":"Google Analytics"})[id]:o.name(id);
      const top=h("div","lv-top"),sc=h("span","bz-score "+(r?(r.score>=75?"pass":r.score>=50?"warn":"fail"):"warn"),r?String(r.score):"–");
      const ti=h("div");add(ti,h("b",null,nm),h("small","bz-mut",r?(GC[id]?"Google":(r.target||""))+" · updated "+ago(r.createdAt):"Not analysed yet"));add(top,sc,ti);c.appendChild(top);
      if(!r){const b=h("button","bz-btn",busy.has(id)?"Reading…":"Analyse now");b.disabled=busy.has(id);b.onclick=()=>refresh(id);c.appendChild(b);g.appendChild(c);continue}
      const ks=h("div","lv-kpis");const kk=kpisFor(r,id);if(GC[id]&&!kk.length&&!open(r,id).length&&!A(r.checks).some(x=>x.category===GC[id])){c.appendChild(h("p","bz-mut","This Google account has no data for this source yet."));const rf0=h("button","bz-btn","Refresh");rf0.onclick=()=>refresh(id);c.appendChild(rf0);g.appendChild(c);continue}
      for(const k of kk.slice(0,exp.has(id)?8:3)){const d=h("div");add(d,h("small",null,k.label),h("b",null,k.value));ks.appendChild(d)}c.appendChild(ks);
      const iss=open(r,id).filter(x=>!v.done[key(x)]).sort((a,b)=>((a.status==="fail"?0:1)-(b.status==="fail"?0:1))||b.impact-a.impact);
      const dn=Object.keys(v.done);
      const sm=h("div","lv-sum");add(sm,h("span",iss.length?"lv-todo":"lv-ok",iss.length?iss.length+" to fix":"Nothing to fix"),dn.length?h("span","lv-done",dn.length+" done"):null);c.appendChild(sm);
      if(!exp.has(id)){for(const x of iss.slice(0,2)){const d=h("p","lv-i");d.textContent=(x.status==="fail"?"● ":"○ ")+x.name;c.appendChild(d)}}
      else{
        const l=h("div","lv-issues");
        for(const x of iss){const d=h("div","lv-issue "+x.status);const body=h("div");add(body,h("b",null,x.name+" — "+x.value),h("p",null,x.fix));
          if(v.reopened.includes(key(x)))body.insertBefore(h("em","lv-again","Still a problem after refresh"),body.firstChild);
          const m=h("button","bz-mini","Mark as fixed");m.onclick=()=>{const s=store(o.uid,id),z=s.get();z.done[key(x)]={at:new Date().toISOString(),manual:true};z.reopened=z.reopened.filter(q=>q!==key(x));s.put(z);draw()};add(d,body,m);l.appendChild(d)}
        if(!iss.length)l.appendChild(h("p","bz-mut","Nothing to fix right now."));c.appendChild(l);
        if(dn.length){const dd=h("details","lv-dn");dd.appendChild(h("summary",null,"Done and dismissed ("+dn.length+")"));
          for(const k of dn){const p=h("p","lv-donei");add(p,h("span","lv-tick","✓ Done"),document.createTextNode(" "+k.split("|")[1]+(v.done[k].manual?"":" (verified on refresh)")));dd.appendChild(p)}c.appendChild(dd)}
        const fr=h("button","bz-btn ghost","Open the full report");fr.onclick=()=>o.open(r);c.appendChild(fr)}
      const bar=h("div","lv-bar"),rf=h("button","bz-btn",busy.has(id)?"Reading…":"Refresh"),ex=h("button","bz-btn ghost",exp.has(id)?"Collapse":"Expand");
      rf.disabled=busy.has(id);rf.onclick=()=>refresh(id);ex.onclick=()=>{exp.has(id)?exp.delete(id):exp.add(id);draw()};add(bar,rf,ex);c.appendChild(bar);
      if(exp.has(id))c.style.gridColumn="1/-1";g.appendChild(c)}}
  return{draw,refresh,o}}
