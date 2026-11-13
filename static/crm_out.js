// Outbound: B2B prospecting, AI cold email, sequences and the smart inbox (LeadForge-style).
export function outbound(X){
const{h,add,api,copy,S,can,mk,formEl,modal,close,msg,setHead,V,label,pill,ago,date,ownerOpts,load,show,A,icon}=X;
const T=(t,s)=>{const e=h("h3",null,t);if(s)e.appendChild(h("small","bz-mut"," "+s));return e};
const card=()=>{const c=h("section","bz-card");c.style.marginBottom="14px";return c};
const busy=async(b,fn)=>{const t=b.textContent;b.disabled=true;b.textContent="Working…";try{await fn()}finally{b.disabled=false;b.textContent=t}};
let SET=null;
const settings=async(f)=>{if(!SET||f)SET=await api("GET","/api/crm/outbound/settings");return SET};
const LABELS=["interested","follow_up","not_interested","out_of_office","unsubscribe","bounce","other"];
const LCOL={interested:"qualified",follow_up:"contacted",not_interested:"unqualified",out_of_office:"new",unsubscribe:"lost",bounce:"lost",other:"new"};
const lpill=l=>pill(LCOL[l]||"new");
const lbl=l=>{const p=h("span","crm-pill "+(LCOL[l]||"new"),label(l));return p};
const stp=s=>{const st=(s||"").toLowerCase();const c=/verified|valid/.test(st)&&!/un/.test(st)?"qualified":/found on website|confidence|entered/.test(st)?"contacted":/no email|cannot/.test(st)?"lost":"new";return h("span","crm-pill "+c,s||"")};
const warn=(p,t)=>{const d=h("p","bz-mut",t);d.style.cssText="border-left:3px solid #fbbf24;padding:6px 10px;margin:8px 0";p.appendChild(d)};

// ---------------------------------------------------------------- cold email draft
async function coldEmail(lead){
  const b=h("div"),s=await settings();
  if(!s.offer){b.appendChild(h("p","bz-err","Tell the AI what you sell first: Outbound > Settings > What you offer."));modal("AI cold email",b);return}
  const out=h("div");b.appendChild(h("p","bz-mut","Writes a short personalised first email and two follow-ups for "+(lead.name||"this lead")+"."));
  const tone=mk("select");for(const t of["friendly and direct","formal","casual","bold and short"])tone.appendChild(mk("option",{value:t,textContent:label(t)}));
  const go=h("button","bz-btn","Write with AI");const row=h("div","crm-row");add(row,tone,go);add(b,row,out);modal("AI cold email",b,true);
  go.onclick=()=>busy(go,async()=>{out.innerHTML="";try{const d=await api("POST","/api/crm/outreach/draft",{leadId:lead.id,tone:tone.value,domain:(lead.notes||"").match(/Website: (\S+)/)?.[1]||""});
    const sj=mk("input",{value:d.subject}),bd=mk("textarea",{rows:8,value:d.body});add(out,h("label",null,"Subject"),sj,h("label",null,"Email"),bd);
    A(d.followups).forEach((f,i)=>{add(out,h("label",null,"Follow-up "+(i+1)),mk("textarea",{rows:4,value:f}))});
    const cp=h("button","bz-mini","Copy email");cp.onclick=()=>copy(sj.value+"\n\n"+bd.value);const mt=h("a","bz-mini","Open in my mail app");mt.href="mailto:"+encodeURIComponent(lead.email||"")+"?subject="+encodeURIComponent(sj.value)+"&body="+encodeURIComponent(bd.value);
    const r2=h("div","crm-row");r2.style.marginTop="10px";add(r2,cp,mt);out.appendChild(r2)}catch(x){msg(out,x.message,1)}})}

// ---------------------------------------------------------------- prospector
const SIZES=[["","Any size"],["1-10","1-10"],["11-50","11-50"],["51-200","51-200"],["201-500","201-500"],["501+","500+"]];
async function prospector(){
  const s=await settings(true);
  setHead("Prospector","Find companies and decision makers, then move the good ones to My Leads");
  const v=V();v.innerHTML="";
  const sc=card();sc.appendChild(T("Find prospects","who is your ideal customer?"));
  const f=formEl([{k:"titles",label:"Job titles (comma separated)",type:"text",wide:1},{k:"industry",label:"Industry"},{k:"location",label:"Location (city or country)"},{k:"size",label:"Company size",type:"select",opts:SIZES},{k:"keywords",label:"Keywords"},{k:"count",label:"How many (max 15)",type:"number"},
    {k:"mode",label:"Data source",type:"select",opts:[["auto","Automatic"],["web","Free AI web search"],["apollo","Apollo database"+(s.apollo?"":" (add key)")]]}],{titles:"CEO, Founder, Marketing Director",count:10,mode:"auto"});
  const go=h("button","bz-btn","Search");go.onclick=()=>busy(go,async()=>{const o=f.get();o.titles=o.titles.split(",").map(x=>x.trim()).filter(Boolean);
    try{const r=await api("POST","/api/crm/prospect/search",o);const m=r.added+" new prospects added ("+r.duplicates+" already had)."+(r.notes||[]).map(n=>" "+n).join("");msg(sc,m);draw()}catch(x){msg(sc,x.message,1)}});
  add(sc,f,go);
  sc.appendChild(h("p","bz-mut",s.apollo||s.hunter?"Keys connected: "+[s.apollo&&"Apollo",s.hunter&&"Hunter"].filter(Boolean).join(", ")+".":"Using the free AI web search: it finds companies on Google and reads their public contact pages. Add Apollo and Hunter keys in Admin for deeper, verified data."));
  v.appendChild(sc);
  const lc=card();lc.appendChild(T("Find one person's email","you know the name and the company"));
  const lf=formEl([{k:"name",label:"Full name",req:1},{k:"domain",label:"Company website",req:1},{k:"title",label:"Job title"}],{});
  const lo=h("div");const l1=h("button","bz-btn ghost","Find email"),l2=h("button","bz-btn","Find and save");
  const doL=(b,save)=>()=>busy(b,async()=>{lo.innerHTML="";try{const o=f2();o.save=save;const r=await api("POST","/api/crm/prospect/lookup",o);
    const t=h("div");for(const c of A(r.candidates)){const row=h("div","crm-row");row.style.margin="4px 0";add(row,h("code",null,c.email),stp(c.status));t.appendChild(row)}
    lo.appendChild(t);if(r.note)lo.appendChild(h("p","bz-mut",r.note));if(r.saved){lo.appendChild(h("p",null,"Saved to your prospects."));draw()}}catch(x){msg(lo,x.message,1)}});
  const f2=()=>lf.get();l1.onclick=doL(l1,false);l2.onclick=doL(l2,true);const lr=h("div","crm-row");add(lr,l1,l2);add(lc,lf,lr,lo);v.appendChild(lc);
  const tc=card();tc.appendChild(T("All prospects","waiting for review, not in your CRM yet"));const hold=h("div");tc.appendChild(hold);v.appendChild(tc);
  let items=[],onlyMail=false;const sel=new Set();
  async function draw(){try{items=A((await api("GET","/api/crm/prospects")).items)}catch(x){hold.innerHTML="";msg(hold,x.message,1);return}paint()}
  function paint(){hold.innerHTML="";const rows=items.filter(p=>!p.promoted&&(!onlyMail||p.email));
    const bar=h("div","crm-bar");const ck=mk("input",{type:"checkbox",checked:onlyMail});ck.onchange=()=>{onlyMail=ck.checked;paint()};const cl=h("label",null);cl.style.cssText="display:flex;gap:6px;align-items:center";add(cl,ck,h("span",null,"Only with an email"));
    const mv=h("button","bz-btn","Move to My Leads"),ms=h("button","bz-btn ghost","Move + start sequence"),dl=h("button","bz-mini","Delete selected");
    mv.onclick=()=>busy(mv,async()=>{await promote(false)});ms.onclick=()=>busy(ms,async()=>{await promote(true)});
    dl.onclick=async()=>{if(!sel.size||!confirm("Delete "+sel.size+" prospects?"))return;await api("POST","/api/crm/prospect/delete",{ids:[...sel]});sel.clear();draw()};
    add(bar,cl,h("span","sp"),h("span","bz-mut",rows.length+" prospects"),can("leads","c")?mv:null,can("leads","c")&&can("leads","e")?ms:null,can("leads","e")?dl:null);hold.appendChild(bar);
    if(!rows.length){hold.appendChild(h("p","bz-mut","No prospects yet. Run a search above."));return}
    const w=h("div","crm-wrap"),t=h("table","crm-t"),hd=h("tr");const all=mk("input",{type:"checkbox"});all.onchange=()=>{rows.forEach(r=>all.checked?sel.add(r.id):sel.delete(r.id));paint()};
    const th0=h("th");th0.appendChild(all);hd.appendChild(th0);for(const c of["Person","Company","Email","Phone","Source",""])hd.appendChild(h("th",null,c));t.appendChild(hd);
    for(const p of rows){const tr=h("tr","row");const c0=h("td"),k=mk("input",{type:"checkbox",checked:sel.has(p.id)});k.onchange=()=>{k.checked?sel.add(p.id):sel.delete(p.id)};c0.appendChild(k);tr.appendChild(c0);
      const pp=h("td");pp.append(p.name||"",h("br"));pp.appendChild(h("small","bz-mut",p.title||""));if(p.linkedin){const a=h("a","ln"," in");a.href=p.linkedin;a.target="_blank";a.rel="noopener";pp.appendChild(a)}
      const em=h("td");em.append(p.email||"",h("br"));if(p.emailStatus)em.appendChild(stp(p.emailStatus));
      const cm=h("td");cm.append(p.company||"",h("br"));cm.appendChild(h("small","bz-mut",[p.industry,p.location].filter(Boolean).join(" · ")));
      const ac=h("td");ac.style.whiteSpace="nowrap";
      if(!p.email&&p.domain&&can("leads","e")){const fb=h("button","bz-mini","Find email");fb.onclick=()=>busy(fb,async()=>{try{const r=await api("POST","/api/crm/prospect/find",{id:p.id});if(!r.found)alert("No email found on their website.");draw()}catch(x){alert(x.message)}});ac.appendChild(fb)}
      if(can("leads","e")){const eb=h("button","bz-mini","Edit");eb.onclick=()=>editP(p);ac.append(" ",eb)}
      add(tr,pp,cm,em,(()=>{const t=h("td");if(p.phone){const a=h("a","ln",p.phone);a.href="tel:"+p.phone.replace(/[^+\d]/g,"");t.appendChild(a)}return t})(),h("td",null,label(p.source)),ac);t.appendChild(tr)}
    w.appendChild(t);hold.appendChild(w)}
  async function promote(thenSeq){const ids=[...sel];if(!ids.length){alert("Tick the prospects you want first.");return}
    const r=await api("POST","/api/crm/prospect/promote",{ids});S.data.leads=null;sel.clear();
    if(thenSeq){await draw();const sq=A((await api("GET","/api/crm/sequences")).items);if(!sq.length){alert(r.created+" leads added. Create a sequence first (Outbound > Sequences), then enroll them.");return}enrollModal(sq,r.leadIds);return}
    alert(r.created+" added to My Leads"+(r.duplicates?" ("+r.duplicates+" were already leads)":"")+".");draw()}
  function editP(p){const b=h("div"),ef=formEl([{k:"name",label:"Name"},{k:"title",label:"Job title"},{k:"company",label:"Company"},{k:"email",label:"Email",type:"email"},{k:"phone",label:"Phone"}],p);
    const sv=h("button","bz-btn","Save");sv.onclick=async()=>{try{await api("POST","/api/crm/prospect/edit",Object.assign({id:p.id},ef.get()));close();draw()}catch(x){msg(b,x.message,1)}};add(b,ef,sv);modal("Edit prospect",b)}
  draw()}

// ---------------------------------------------------------------- sequences
function enrollModal(seqs,leadIds){
  const b=h("div"),sq=mk("select");for(const x of seqs)sq.appendChild(mk("option",{value:x.id,textContent:x.name}));
  b.appendChild(h("p","bz-mut","Leads without an email address or that unsubscribed are skipped automatically. The first email goes out inside the sequence's sending window."));
  const go=h("button","bz-btn","Start sequence for "+leadIds.length+" leads");go.onclick=()=>busy(go,async()=>{try{const r=await api("POST","/api/crm/sequence/enroll",{seqId:sq.value,leadIds});b.innerHTML="";b.appendChild(h("p",null,r.added+" enrolled, "+r.skipped+" skipped, "+r.noEmail+" had no email."))}catch(x){msg(b,x.message,1)}});
  add(b,sq,go);modal("Start a sequence",b)}
async function pickLeads(seq){
  await load("leads");const b=h("div"),q=mk("input",{placeholder:"Search leads…"}),box=h("div");box.style.cssText="max-height:46vh;overflow:auto;margin:10px 0";const sel=new Set();
  const paint=()=>{box.innerHTML="";const t=q.value.toLowerCase();for(const l of A(S.data.leads).filter(l=>l.email&&(!t||JSON.stringify(l).toLowerCase().includes(t))).slice(0,300)){
    const r=h("label","crm-row");r.style.cssText="margin:2px 0;gap:8px";const k=mk("input",{type:"checkbox",checked:sel.has(l.id)});k.onchange=()=>{k.checked?sel.add(l.id):sel.delete(l.id)};add(r,k,h("span",null,l.name+" · "+l.email+(l.company?" · "+l.company:"")));box.appendChild(r)}
    if(!box.firstChild)box.appendChild(h("p","bz-mut","No leads with an email address."))};q.oninput=paint;paint();
  const go=h("button","bz-btn","Start sequence");go.onclick=()=>busy(go,async()=>{if(!sel.size){alert("Tick at least one lead.");return}try{const r=await api("POST","/api/crm/sequence/enroll",{seqId:seq.id,leadIds:[...sel]});close();alert(r.added+" enrolled, "+r.skipped+" skipped.");show("sequences")}catch(x){msg(b,x.message,1)}});
  add(b,q,box,go);modal("Enroll leads in "+seq.name,b)}
async function seqEditor(seq){
  const s=await settings();const b=h("div"),d=Object.assign({name:"",active:false,windowStart:9,windowEnd:17,weekdaysOnly:true,dailyLimit:30,steps:[{delayDays:0,subject:"",body:""},{delayDays:3,subject:"Re: ",body:""}]},seq||{});
  d.steps=JSON.parse(JSON.stringify(d.steps));
  const f=formEl([{k:"name",label:"Sequence name",req:1,wide:1},{k:"windowStart",label:"Send from hour (0-23)",type:"number"},{k:"windowEnd",label:"Send until hour (1-24)",type:"number"},{k:"dailyLimit",label:"Max emails per day (1-200)",type:"number"},{k:"weekdaysOnly",label:"Weekdays only",type:"select",opts:[["1","Yes"],["","No, include weekends"]]}],Object.assign({},d,{weekdaysOnly:d.weekdaysOnly?"1":""}));
  const act=mk("input",{type:"checkbox",checked:d.active}),al=h("label","crm-row");al.style.gap="8px";add(al,act,h("span",null,"Active (emails are sent automatically)"));
  const sh=h("div");
  const paint=()=>{sh.innerHTML="";d.steps.forEach((st,i)=>{const c=card();c.appendChild(h("b",null,i===0?"Email 1":"Follow-up "+i));
      const dl=mk("input",{type:"number",min:0,max:60,value:st.delayDays});dl.oninput=()=>st.delayDays=+dl.value||0;const sj=mk("input",{value:st.subject});sj.oninput=()=>st.subject=sj.value;const bd=mk("textarea",{rows:5,value:st.body});bd.oninput=()=>st.body=bd.value;
      add(c,h("label",null,i===0?"Wait this many days after enrolling":"Wait this many days after the previous email"),dl,h("label",null,"Subject"),sj,h("label",null,"Message"),bd);
      const tb=h("button","bz-mini","Send this email to me as a test");tb.style.marginRight="8px";tb.onclick=()=>busy(tb,async()=>{try{const r=await api("POST","/api/crm/sequence/test",{subject:st.subject,body:st.body});msg(c,"Test sent to "+r.sentTo+". Check your inbox and spam folder.")}catch(x){msg(c,x.message,1)}});c.appendChild(tb);
      if(d.steps.length>1){const rm=h("button","bz-mini","Remove");rm.onclick=()=>{d.steps.splice(i,1);paint()};c.appendChild(rm)}sh.appendChild(c)})};paint();
  const EX=[{delayDays:0,subject:"Quick question, {{first_name}}",body:"Hi {{first_name}},\n\nI'm {{sender}} from TechET Solution. We help investors and companies in Dubai find the right apartments and villas, from shortlisting to viewings, payment plans and handover.\n\nIs property in Dubai something {{company}} is looking at this year?\n\nBest,\n{{sender}}"},{delayDays:3,subject:"Re: Quick question, {{first_name}}",body:"Hi {{first_name}},\n\nA short follow-up. Right now we are seeing strong rental yields in areas like Business Bay and JVC, and I can send 3 options that match your budget in one short email.\n\nWould that be useful?\n\n{{sender}}"},{delayDays:5,subject:"Should I close this?",body:"Hi {{first_name}},\n\nI don't want to fill your inbox. If Dubai property isn't a priority, just tell me and I will stop. If it is, reply with your budget and preferred area and I will send options.\n\n{{sender}}"}];
  const ex=h("button","bz-mini","Load example (Dubai real estate)");ex.onclick=()=>{d.steps=JSON.parse(JSON.stringify(EX));paint()};
  const addB=h("button","bz-mini","+ Add follow-up");addB.onclick=()=>{if(d.steps.length<8){d.steps.push({delayDays:4,subject:"Re: ",body:""});paint()}};
  const ai=h("button","bz-btn ghost","Write with AI");ai.onclick=()=>busy(ai,async()=>{try{const r=await api("POST","/api/crm/outreach/draft",{template:true});d.steps=[{delayDays:0,subject:r.subject,body:r.body}].concat(A(r.followups).map((t,i)=>({delayDays:3+i*4,subject:"Re: "+r.subject,body:t})));paint()}catch(x){msg(b,x.message,1)}});
  const sv=h("button","bz-btn","Save sequence");sv.onclick=()=>busy(sv,async()=>{try{const o=f.get();o.weekdaysOnly=!!o.weekdaysOnly;await api("POST","/api/crm/sequence/save",{id:seq?seq.id:"",name:o.name,active:act.checked,steps:d.steps,windowStart:o.windowStart,windowEnd:o.windowEnd,weekdaysOnly:o.weekdaysOnly,dailyLimit:o.dailyLimit});close();show("sequences")}catch(x){msg(b,x.message,1)}});
  b.appendChild(h("p","bz-mut","Use {{first_name}}, {{company}}, {{title}} and {{sender}} in subjects and messages. An unsubscribe link and your address are added automatically. A person who replies is removed from the sequence."));
  if(!s.smtp)warn(b,"No SMTP mailbox is connected. Cold email should go from your own mailbox (SMTP_HOST, SMTP_USER, SMTP_PASS in Admin); transactional services like Resend forbid cold outreach.");
  add(b,f,al,sh);const r=h("div","crm-row");add(r,addB,ex,ai,sv);b.appendChild(r);modal(seq?"Edit sequence":"New sequence",b,true)}
async function enrollView(seq){
  const b=h("div");try{const r=A((await api("GET","/api/crm/sequence/enrollments?seqId="+seq.id)).items);
    if(!r.length)b.appendChild(h("p","bz-mut","Nobody is enrolled yet."));else{const w=h("div","crm-wrap"),t=h("table","crm-t"),hd=h("tr");for(const c of["Lead","Email","Status","Next email","Opens",""])hd.appendChild(h("th",null,c));t.appendChild(hd);
      for(const e of r){const tr=h("tr");const st=h("span","crm-pill "+({replied:"qualified",done:"won",active:"new",error:"lost",bounced:"lost",unsubscribed:"lost"}[e.status]||"contacted"),label(e.status)+(e.lastErr?" ⚠":""));st.title=e.lastErr||"";
        const ac=h("td");if(e.status==="active"&&can("leads","e")){const x=h("button","bz-mini","Stop");x.onclick=async()=>{await api("POST","/api/crm/sequence/stop",{id:e.id});close();enrollView(seq)};ac.appendChild(x)}
        const stc=h("td");stc.appendChild(st);add(tr,h("td",null,e.name+(e.company?" · "+e.company:"")),h("td",null,e.email),stc,h("td",null,e.status==="active"&&e.nextAt?"Email "+(e.step+1)+" · "+date(e.nextAt):""),h("td",null,String(e.opens)),ac);t.appendChild(tr)}w.appendChild(t);b.appendChild(w)}}catch(x){msg(b,x.message,1)}
  modal("People in "+seq.name,b,true)}
async function sequences(){
  setHead("Sequences","Automatic multi-step emails from your own mailbox",[can("leads","e")?(()=>{const n=h("button","bz-btn","+ New sequence");n.onclick=()=>seqEditor(null);return n})():null]);
  const v=V();v.innerHTML="";const s=await settings(true);
  if(!s.smtp)warn(v,s.mailReady?"Only Resend is connected. Resend does not allow cold outreach: add your own mailbox (SMTP_HOST, SMTP_USER, SMTP_PASS in Admin) before turning a sequence on.":"Email sending is not set up. Add MAIL_FROM and SMTP_HOST, SMTP_USER, SMTP_PASS in Admin.");
  if(!s.address)warn(v,"Add your company address in Outbound > Settings: it is required in every cold email.");
  const r=A((await api("GET","/api/crm/sequences")).items);if(!r.length){v.appendChild(h("p","bz-mut","No sequences yet. Create one, or let the AI write it."));return}
  for(const q of r){const c=card(),st=q.stats||{};const top=h("div","crm-row");top.style.justifyContent="space-between";
    add(top,h("b",null,q.name),h("span","crm-pill "+(q.active?"qualified":"new"),q.active?"Active":"Paused"));c.appendChild(top);
    const n=x=>String(Math.max(0,Math.round(x||0)));
    c.appendChild(h("p","bz-mut",A(q.steps).length+" emails · "+(q.windowStart||9)+":00-"+(q.windowEnd||17)+":00 · up to "+(q.dailyLimit||30)+"/day · enrolled "+n(st.enrolled)+" · sent "+n(st.sent)+" · opened "+n(st.opens)+" · replied "+n(st.replies)+" · unsubscribed "+n(st.unsub)));
    const row=h("div","crm-row");const en=h("button","bz-btn","Enroll leads"),vw=h("button","bz-mini","People"),ed=h("button","bz-mini","Edit"),dl=h("button","bz-mini","Delete");
    en.onclick=()=>pickLeads(q);vw.onclick=()=>enrollView(q);ed.onclick=()=>seqEditor(q);dl.onclick=async()=>{if(!confirm("Delete this sequence and its enrollments?"))return;await api("POST","/api/crm/sequence/delete",{id:q.id});show("sequences")};
    add(row,can("leads","e")?en:null,vw,can("leads","e")?ed:null,can("leads","d")?dl:null);c.appendChild(row);v.appendChild(c)}
  v.appendChild(h("p","bz-mut","Open counts are approximate: some mail apps load images automatically, others block them."))}

// ---------------------------------------------------------------- smart inbox
async function inbox(){
  setHead("Smart inbox","Replies sorted by the AI so you answer the right people first");
  const v=V();v.innerHTML="";const s=await settings(true);let items=A((await api("GET","/api/crm/inbox")).items),f="";
  const ac=card();ac.appendChild(T("Add a reply","paste an email you received"));
  const af=formEl([{k:"from",label:"From (email)",req:1},{k:"subject",label:"Subject"},{k:"text",label:"What they wrote",type:"textarea",rows:4,wide:1}],{});const ab=h("button","bz-btn","Classify it");
  ab.onclick=()=>busy(ab,async()=>{try{const r=await api("POST","/api/crm/inbox/add",af.get());show("inbox")}catch(x){msg(ac,x.message,1)}});add(ac,af,ab);
  const hint=h("p","bz-mut","To fill this automatically, forward replies to this address (Mailgun route, Postmark inbound, Zapier or Make with a Gmail trigger, or a mailbox forwarding rule that posts JSON): "+(s.inboundUrl||""));
  const cp=h("button","bz-mini","Copy inbound URL");cp.onclick=()=>copy(s.inboundUrl);ac.append(hint,cp);v.appendChild(ac);
  const chips=h("div","bx-chips");chips.style.margin="8px 0";const list=h("div");
  const cnt=l=>items.filter(i=>!l||i.label===l).length;
  const paint=()=>{chips.innerHTML="";for(const l of[""].concat(LABELS)){const b=h("button","bx-chip"+(f===l?" on":""),(l?label(l):"All")+" "+cnt(l));b.onclick=()=>{f=l;paint()};chips.appendChild(b)}
    list.innerHTML="";const rows=items.filter(i=>!f||i.label===f);if(!rows.length){list.appendChild(h("p","bz-mut","No replies here yet."));return}
    for(const i of rows){const c=card();const top=h("div","crm-row");top.style.cssText+="justify-content:space-between;gap:8px";
      const who=h("b",null,(i.fromName||i.from)+(i.leadName?"":" "));if(!i.leadId)who.appendChild(h("small","bz-mut"," · not in your leads"));
      const sl=mk("select");for(const l of LABELS)sl.appendChild(mk("option",{value:l,textContent:label(l),selected:l===i.label}));sl.onchange=async()=>{await api("POST","/api/crm/inbox/handle",{id:i.id,label:sl.value});i.label=sl.value;paint()};
      add(top,who,h("span","crm-row"),lbl(i.label),can("leads","e")?sl:null);c.appendChild(top);
      c.appendChild(h("p",null,(i.subject?i.subject+" — ":"")+(i.summary||"")));c.appendChild(h("small","bz-mut",ago(i.receivedAt)+" · "+i.from+(i.handled?" · handled":"")));
      const det=h("details");det.appendChild(h("summary",null,"Read the reply"));const pre=h("pre",null,i.text||"");pre.style.cssText="white-space:pre-wrap;font:inherit";det.appendChild(pre);c.appendChild(det);
      if(can("leads","e")&&!["bounce","unsubscribe"].includes(i.label)){const ta=mk("textarea",{rows:3,value:i.suggest||"",placeholder:"Write your reply…"});const send=h("button","bz-btn","Send reply");
        send.onclick=()=>busy(send,async()=>{try{await api("POST","/api/crm/inbox/reply",{id:i.id,body:ta.value,subject:"Re: "+(i.subject||"").replace(/^re:\s*/i,"")});i.handled=true;paint()}catch(x){msg(c,x.message,1)}});
        c.append(h("label",null,i.suggest?"AI suggested reply (edit before sending)":"Reply"),ta,send)}
      const row=h("div","crm-row");row.style.marginTop="6px";if(can("leads","e")){const hd=h("button","bz-mini",i.handled?"Mark as new":"Mark as handled");hd.onclick=async()=>{await api("POST","/api/crm/inbox/handle",{id:i.id,handled:!i.handled});i.handled=!i.handled;paint()};const dx=h("button","bz-mini","Delete");dx.onclick=async()=>{await api("POST","/api/crm/inbox/handle",{id:i.id,delete:true});items=items.filter(x=>x!==i);paint()};add(row,hd,dx)}
      if(i.leadId){const lk=h("button","bz-mini","Open lead");lk.onclick=async()=>{await load("leads",true);const l=A(S.data.leads).find(x=>x.id===i.leadId);if(l)X.openRecord("leads",l)};row.appendChild(lk)}
      c.appendChild(row);list.appendChild(c)}};
  paint();v.append(chips,list)}

// ---------------------------------------------------------------- settings
async function obsettings(){
  setHead("Outbound settings","What you sell, who you are, and what is connected");const v=V();v.innerHTML="";const s=await settings(true);const c=card();
  const ck=(ok,t)=>h("p",ok?null:"bz-mut",(ok?"✓ ":"○ ")+t);
  add(c,T("Connection status"),ck(s.ai,"AI (Gemini) for drafts and reply sorting"),ck(s.smtp,"Own mailbox (SMTP) for cold email"),ck(s.baseUrl,"Public app address (BASE_URL) for unsubscribe and open tracking"),ck(!!s.address,"Company postal address in every email"),ck(s.apollo,"Apollo key (optional, B2B database)"),ck(s.hunter,"Hunter key (optional, email finder and verifier)"));v.appendChild(c);
  const fc=card();fc.appendChild(T("Your outreach profile"));
  const f=formEl([{k:"offer",label:"What you offer (the AI uses this to write emails)",type:"textarea",rows:4,wide:1},{k:"fromName",label:"Your name (signature)"},{k:"company",label:"Company name"},{k:"address",label:"Company postal address",wide:1},{k:"replyTo",label:"Reply-to email",type:"email"},{k:"tz",label:"Time zone, hours from UTC (Dubai = 4)",type:"number"}],s);
  const sv=h("button","bz-btn","Save");sv.disabled=!can("integrations","e");sv.onclick=()=>busy(sv,async()=>{try{const o=f.get();await api("POST","/api/crm/outbound/settings",o);await settings(true);msg(fc,"Saved.")}catch(x){msg(fc,x.message,1)}});add(fc,f,sv);v.appendChild(fc);
  const tc=card();add(tc,T("Doing cold email safely"),h("p","bz-mut","Warm up a new mailbox: start at 20-30 emails a day and raise slowly. Use a separate domain, not your main one. Set up SPF, DKIM and DMARC for the sending domain. Only email business contacts who could plausibly be interested, and honour unsubscribes (done automatically here). Laws such as CAN-SPAM, GDPR and UAE data rules apply to you as the sender."));v.appendChild(tc)}

return{views:{prospects:prospector,sequences,inbox,obsettings},coldEmail,enrollModal}}
