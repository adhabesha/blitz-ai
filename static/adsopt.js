// Audience targeting and campaign optimisation, applied to your own Meta ad account.
// Every change is previewed (before -> after), capped, logged on the server and can be undone.
import{h,add,api}from"./ui.js";
const fmt=(v,c)=>Number(v).toLocaleString(undefined,{maximumFractionDigits:2})+" "+c;
const warn=c=>c.appendChild(h("p","bz-note","This changes your live Meta ads. You will see exactly what changes before anything is applied, and you can undo it."));
const ages=(a,b,adv)=>"ages "+a+"–"+b+(adv?", Advantage+ audience on":"");

export function scaleFix(c,ctx){
  warn(c);const box=h("div");c.appendChild(box);const st=h("p","bz-mut");
  const body={type:"scale",id:ctx.id,pct:ctx.pct||20,account:ctx.account};
  box.textContent="Checking the current budget…";
  api("POST","/api/ads/optimize",Object.assign({dry:true},body)).then(r=>{
    box.innerHTML="";for(const x of r.rows)box.appendChild(h("p",null,x.name+": "+fmt(x.from,r.currency)+" per day → "+fmt(x.to,r.currency)+" per day"));
    const go=h("button","bz-btn","Raise the budget "+body.pct+"%");
    go.onclick=async()=>{go.disabled=true;st.textContent="Applying…";try{const d=await api("POST","/api/ads/optimize",body);st.textContent="Done. Check results in 3 days before raising again.";go.hidden=true;
      const un=h("button","bz-btn ghost","Undo");un.onclick=async()=>{un.disabled=true;try{for(const u of d.undo)await api("POST","/api/ads/action",{type:"budget",id:u.id,value:u.value,account:ctx.account});st.textContent="Budget restored.";un.hidden=true}catch(e){st.textContent=e.message;un.disabled=false}};box.appendChild(un)}catch(e){st.textContent=e.message;go.disabled=false}};
    box.appendChild(go)}).catch(e=>{box.textContent=e.message});
  c.appendChild(st)}

export function audienceFix(c,ctx){
  warn(c);const list=h("div");c.appendChild(list);list.textContent="Reading your ad sets and age results (last 30 days)…";
  const q="/api/ads/targeting?account="+encodeURIComponent(ctx.account||"")+(ctx.id?"&id="+encodeURIComponent(ctx.id):"");
  api("GET",q).then(d=>{list.innerHTML="";const cur=d.currency;
    if(!d.adsets.length){list.textContent="No ad sets found for this account.";return}
    for(const a of d.adsets.slice(0,6)){
      const card=h("div","bz-card");card.style.margin="10px 0";
      add(card,h("h4",null,a.name),h("p","bz-mut",(a.status||"").toLowerCase()+" · "+(a.budget?fmt(a.budget,cur)+" per day":"budget set on the campaign")+" · now "+ages(a.ageMin,a.ageMax,a.advantage)+(a.countries&&a.countries.length?" · "+a.countries.join(", "):"")));
      if(a.segments&&a.segments.length){const mx=Math.max(...a.segments.map(s=>s.ctr),0.01);
        for(const s of a.segments){const row=h("div");row.style.cssText="display:grid;grid-template-columns:54px 1fr 210px;gap:8px;align-items:center;font-size:13px;margin:3px 0";const bar=h("div");bar.style.cssText="height:8px;border-radius:6px;background:var(--accent);width:"+Math.max(4,s.ctr/mx*100)+"%";const wrap=h("div");wrap.appendChild(bar);
          add(row,h("span",null,s.age),wrap,h("span","bz-mut",s.ctr.toFixed(2)+"% CTR"+(s.results?" · "+s.results+" results"+(s.cpa?" · "+fmt(s.cpa,cur)+" each":""):"")));card.appendChild(row)}}
      if(a.why)card.appendChild(h("p",null,"Suggestion: "+a.why));
      const out=h("div");const acts=h("div","apply-row");
      const run=(label,opts)=>async()=>{out.innerHTML="";out.textContent="Checking…";
        try{const pv=await api("POST","/api/ads/optimize",Object.assign({id:a.id,account:ctx.account,dry:true},opts));
          out.innerHTML="";out.appendChild(h("p",null,(opts.type==="test"?"A paused copy of this ad set will be created with ":"This ad set changes from ")+(opts.type==="test"?ages(pv.after.ageMin,pv.after.ageMax,pv.after.advantage):ages(pv.before.ageMin,pv.before.ageMax,pv.before.advantage)+" to "+ages(pv.after.ageMin,pv.after.ageMax,pv.after.advantage))+"."));
          const ok=h("button","bz-btn",opts.type==="test"?"Create the test ad set":"Apply now"),msg=h("p","bz-mut");
          ok.onclick=async()=>{ok.disabled=true;msg.textContent="Applying…";try{const r=await api("POST","/api/ads/optimize",Object.assign({id:a.id,account:ctx.account},opts));ok.hidden=true;
            if(opts.type==="test"){msg.textContent="Created a paused copy. It spends nothing until you start it.";const go=h("button","bz-btn","Start the test");go.onclick=async()=>{go.disabled=true;try{await api("POST","/api/ads/optimize",{type:"start",id:r.id,account:ctx.account});msg.textContent="The test is running. Compare it with the original after 5–7 days.";go.hidden=true}catch(e){msg.textContent=e.message;go.disabled=false}};out.appendChild(go)}
            else{msg.textContent="Done.";const un=h("button","bz-btn ghost","Undo");un.onclick=async()=>{un.disabled=true;try{await api("POST","/api/ads/optimize",{type:"audience",id:a.id,account:ctx.account,ageMin:r.before.ageMin||18,ageMax:r.before.ageMax||65,advantage:!!r.before.advantage});msg.textContent="Restored the previous audience.";un.hidden=true}catch(e){msg.textContent=e.message;un.disabled=false}};out.appendChild(un)}}catch(e){msg.textContent=e.message;ok.disabled=false}};
          add(out,ok,msg)}catch(e){out.textContent=e.message}};
      const sug=h("button","bz-btn","Apply ages "+a.sugMin+"–"+a.sugMax),tst=h("button","bz-btn ghost","Test it first (new paused ad set)"),br=h("button","bz-btn ghost","Go broad: 18–65 + Advantage+");
      sug.onclick=run("",{type:"audience",ageMin:a.sugMin,ageMax:a.sugMax,advantage:false});tst.onclick=run("",{type:"test",ageMin:a.sugMin,ageMax:a.sugMax,advantage:false});br.onclick=run("",{type:"audience",ageMin:18,ageMax:65,advantage:true});
      add(acts,sug,tst,br);add(card,acts,out);list.appendChild(card)}
  }).catch(e=>{list.textContent=e.message})}
