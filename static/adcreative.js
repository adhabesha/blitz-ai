import{h,add,api,copy,copyBox}from"./ui.js";

// Ad creative: brief, video scripts and finished ad copy for paid social.
let ui={},out;
const lab=(t,el,cls)=>{const l=h("label",cls||null,t);l.appendChild(el);return l};
const inp=(ph,max)=>{const i=h("input");if(ph)i.placeholder=ph;if(max)i.maxLength=max;return i};
const sel=(opts)=>{const s=h("select");for(const[v,l]of opts){const o=h("option",null,l);o.value=v;s.appendChild(o)}return s};
const area=(ph,rows)=>{const t=h("textarea");t.rows=rows||2;t.placeholder=ph;return t};

export function mountAdCreative(root,getWinners){
  root.innerHTML="";
  const w=h("div","wide");root.appendChild(w);
  w.appendChild(h("p","bz-note","Writes a creative brief, timed video scripts and finished ad copy. AI writes the draft: check every claim, price and result before you run it."));
  ui.product=inp("What are you advertising? e.g. handmade leather bags for men",300);
  ui.offer=inp("Offer or promo (optional), e.g. 20% off this week, free delivery",200);
  ui.audience=inp("Who is it for? e.g. women 25-40 in Dubai who want modest fashion",250);
  ui.platform=sel([["meta","Meta feed + Reels"],["reels","Instagram Reels"],["tiktok","TikTok"],["stories","Stories"]]);
  ui.objective=sel([["Sales (maximise ROAS)","Sales (ROAS)"],["Leads (lowest CPA)","Leads (CPA)"],["Traffic","Traffic"],["Awareness","Awareness"]]);
  ui.tone=sel([["Direct and confident","Direct and confident"],["Friendly and conversational","Friendly"],["Luxury and calm","Premium"],["Funny and playful","Playful"],["Urgent","Urgent"]]);
  ui.count=sel([["2","2 concepts"],["3","3 concepts"],["4","4 concepts"],["5","5 concepts"]]);ui.count.value="3";
  ui.country=inp("Market, e.g. UAE, Ethiopia",60);
  ui.proof=area("Real proof points only: reviews, number of customers, awards, guarantees, results (optional)");
  ui.winners=area("Winning ads to build on (optional). Use the button to pull them from your Campaigns managed report.");
  ui.pull=h("button","bz-btn ghost","Pull my winning ads");
  ui.go=h("button","bz-btn","Write brief, scripts and ads");ui.msg=h("p","bz-mut");out=h("div");
  ui.pull.onclick=()=>{const t=getWinners&&getWinners();if(t){ui.winners.value=t;ui.msg.textContent="Winning ads added."}else ui.msg.textContent="No Campaigns managed report yet. Open Run ads, run the Campaigns managed report, then try again."};
  ui.go.onclick=run;
  const f=h("div","form");
  add(f,lab("Product or service",ui.product,"wide"),lab("Offer",ui.offer,"wide"),lab("Audience",ui.audience,"wide"),lab("Platform",ui.platform),lab("Goal",ui.objective),lab("Tone",ui.tone),lab("How many concepts",ui.count),lab("Market",ui.country),lab("Proof points",ui.proof,"wide"),lab("Winning ads to build on",ui.winners,"wide"));
  add(w,f,ui.pull,h("span",null," "),ui.go,ui.msg,out);
  return refresh;
}
// Called every time the tab opens: picks up a draft sent from a report (e.g. "brief 3 new ads from the winner").
export function refresh(){
  const d=window.__bzCreative;if(!d)return;window.__bzCreative=null;
  if(d.product)ui.product.value=d.product;
  if(d.winners)ui.winners.value=d.winners;
  ui.msg.textContent="Started from your winning ad. Add the offer and audience, then write the ads.";
}

async function run(){
  if(ui.product.value.trim().length<3){ui.msg.textContent="Describe what you are advertising.";return}
  ui.go.disabled=true;ui.msg.textContent="Writing the brief, scripts and ads (about 20 seconds)…";out.innerHTML="";
  try{
    const r=await api("POST","/api/adcreative",{product:ui.product.value,offer:ui.offer.value,audience:ui.audience.value,platform:ui.platform.value,objective:ui.objective.value,tone:ui.tone.value,count:parseInt(ui.count.value)||3,country:ui.country.value,proof:ui.proof.value,winners:ui.winners.value});
    ui.msg.textContent="Done. Edit anything, then copy it or send an ad to Run ads.";draw(r);
  }catch(e){ui.msg.textContent=e.message}
  ui.go.disabled=false;
}
const list=a=>(a||[]).filter(Boolean).map(x=>"- "+x).join("\n");
function briefText(b){
  return["CREATIVE BRIEF","Objective: "+(b.objective||""),"Audience: "+(b.audience||""),"Insight: "+(b.insight||""),"Key message: "+(b.keyMessage||""),"Proof points:\n"+list(b.proofPoints),"Mandatories:\n"+list(b.mandatories),"Avoid:\n"+list(b.avoid),"Deliverables:\n"+list(b.deliverables),"Success metrics:\n"+list(b.successMetrics),"Test plan: "+(b.testPlan||"")].join("\n\n");
}
function scriptText(c){
  return(c.script||[]).map(s=>"["+(s.time||"")+"]\nVisual: "+(s.visual||"")+"\nVoiceover: "+(s.voiceover||"")+"\nOn-screen text: "+(s.onScreen||"")).join("\n\n");
}
function draw(r){
  out.innerHTML="";
  const b=r.brief||{},bc=h("div","bz-card");bc.style.marginTop="16px";
  bc.appendChild(h("h3",null,"Creative brief"));
  const bt=copyBox("Brief (copy and send to your designer or creator)",briefText(b),true);bt.ta.rows=14;bc.appendChild(bt);out.appendChild(bc);
  (r.concepts||[]).forEach((c,i)=>{
    const card=h("div","bz-card");card.style.marginTop="16px";
    card.appendChild(h("h3",null,"Concept "+(i+1)+": "+(c.angle||"")));
    if(c.hypothesis)card.appendChild(h("p","bz-mut","Test: "+c.hypothesis));
    card.appendChild(copyBox("Hook (first 3 seconds)",c.hook||"",true));
    const st=copyBox("Video script",scriptText(c),true);st.ta.rows=Math.min(Math.max((c.script||[]).length*4,6),18);card.appendChild(st);
    card.appendChild(h("h4",null,"Finished ad"));
    const g=h("div","bz-two");
    const head=copyBox("Headline (max 40)",(c.adCopy||{}).headline||"",true),prim=copyBox("Primary text (max 125)",(c.adCopy||{}).primary||"",true);
    add(g,head,prim);card.appendChild(g);
    const desc=copyBox("Description (max 30)",(c.adCopy||{}).description||"",true);card.appendChild(desc);
    card.appendChild(h("p","bz-mut","Button: "+((c.adCopy||{}).cta||"LEARN_MORE").replace("_"," ")));
    if(c.staticIdea)card.appendChild(copyBox("Static image ad idea",c.staticIdea,true));
    const row=h("div","apply-row"),all=h("button","bz-btn ghost","Copy whole concept"),use=h("button","bz-btn","Use in Run ads");
    all.onclick=()=>copy("CONCEPT "+(i+1)+": "+(c.angle||"")+"\nHook: "+(c.hook||"")+"\n\n"+st.ta.value+"\n\nHeadline: "+head.ta.value+"\nPrimary text: "+prim.ta.value+"\nDescription: "+desc.ta.value+"\nButton: "+((c.adCopy||{}).cta||"")+(c.staticIdea?"\n\nStatic idea: "+c.staticIdea:""),all);
    use.onclick=()=>{window.__bzAdDraft={head:head.ta.value.slice(0,40),prim:prim.ta.value.slice(0,125),desc:desc.ta.value.slice(0,30),cta:(c.adCopy||{}).cta||"LEARN_MORE",topic:ui.product.value};
      document.dispatchEvent(new CustomEvent("bz-goto",{detail:{kind:"runads"}}))};
    add(row,all,use);use.style.marginLeft="8px";card.appendChild(row);
    card.appendChild(h("p","bz-mut","Use in Run ads fills the headline, text and button of a new campaign. It is created paused, and you still add the image and budget."));
    out.appendChild(card);
  });
}
