import{h,add,api,copyBox}from"./ui.js";

const TYPES={
  short:{label:"Short / Reel",hint:"Vertical video (9:16), up to about 3 minutes. Goes to YouTube Shorts, TikTok, Facebook Reels and Instagram Reels.",accept:"video/mp4,video/quicktime",plats:["youtube","tiktok","facebook","instagram","telegram"]},
  video:{label:"Normal video",hint:"Landscape or square video. Goes to YouTube, Facebook and Instagram (Instagram publishes all videos as Reels).",accept:"video/mp4,video/quicktime",plats:["youtube","facebook","instagram","telegram"]},
  image:{label:"Image",hint:"JPG works everywhere (Instagram does not accept PNG through its API). Goes to Facebook and Instagram.",accept:"image/jpeg,image/png",plats:["facebook","instagram","telegram"]}
};
const PN={youtube:"YouTube",facebook:"Facebook Page",instagram:"Instagram",tiktok:"TikTok",telegram:"Telegram"};
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
let auth,conn={},type="short",plats=new Set(),file=null,ui={},meta=null,tt=null,ttInfo=null;
function verdict(p){
  if(p==="telegram"&&file){const mb=file.size/1048576;if(type==="image"&&mb>10)return{block:"photo over 10 MB"};if(type!=="image"&&mb>50)return{block:"video over 50 MB"};return{}}
  if(!meta)return{};
  const{w,h,d}=meta,r=w/h,t=type;
  if(type==="image"){
    if(p==="instagram"&&file&&/\.png$/i.test(file.name))return{block:"needs a JPG image"};
    if(p==="instagram"&&(r<0.8||r>1.91))return{block:"image ratio must be between 4:5 and 1.91:1"};
    return{};
  }
  if(p==="youtube"&&t==="short"){if(d>180)return{block:"Shorts must be 3 min or shorter"};if(w>h)return{block:"Shorts must be vertical or square"};return{note:"Posted as a YouTube Short (#Shorts added)"}}
  if(p==="instagram"){if(d<3)return{block:"Reels need at least 3 seconds"};if(t==="short"&&w>h)return{block:"Reels must be vertical (9:16)"};if(w>h)return{note:"Posted as a Reel with bars (landscape)"};if(Math.abs(r-9/16)>0.07)return{note:"Not exactly 9:16: may be cropped"};return{note:"Posted as an Instagram Reel"}}
  if(p==="tiktok"){if(d<3)return{block:"needs at least 3 seconds"};if(d>600)return{block:"must be 10 minutes or shorter"};if(w>h)return{block:"must be vertical (9:16)"};return{note:"Posted to TikTok (private until TikTok approves ficx)"}}
  if(p==="facebook"&&t==="short"){const ok=w<=h&&d>=3&&d<=90;return{note:ok?"Posted as a Facebook Reel":"Posted as a normal Facebook video (Reels need vertical 9:16, 3-90 s)"}}
  return{};
}

export function mountPublish(root,a){
  auth=a;root.innerHTML="";
  const w=h("div","wide");root.appendChild(w);
  w.appendChild(h("p","bz-mut","One upload, many platforms. Pick what you are posting, choose the platforms, and ficx sends it to each connected account."));
  ui.seg=h("div","seg3");
  for(const[k,t]of Object.entries(TYPES)){const b=h("button",null,t.label);b.dataset.t=k;b.onclick=()=>{type=k;file=null;meta=null;ui.file.value="";ui.prev.innerHTML="";drawType()};ui.seg.appendChild(b)}
  ui.hint=h("p","bz-mut");
  ui.file=h("input");ui.file.type="file";
  ui.prev=h("div","pub-prev");
  ui.file.onchange=()=>{file=ui.file.files[0]||null;meta=null;ui.prev.innerHTML="";drawPlats();if(file){const u=URL.createObjectURL(file);const el=document.createElement(type==="image"?"img":"video");el.src=u;
    if(type==="image")el.onload=()=>{meta={w:el.naturalWidth,h:el.naturalHeight,d:0};drawPlats()};
    else{el.controls=true;el.muted=true;el.onloadedmetadata=()=>{meta={w:el.videoWidth,h:el.videoHeight,d:el.duration};drawPlats()}}
    ui.prev.appendChild(el)}};
  ui.title=h("input");ui.title.maxLength=100;ui.title.placeholder="Title (YouTube and Facebook)";
  ui.cap=h("textarea");ui.cap.rows=6;ui.cap.maxLength=4000;ui.cap.placeholder="Caption / description. Add hashtags at the end.";
  ui.topic=h("input");ui.topic.placeholder="What is it about? (for AI caption)";
  ui.ai=h("button","bz-btn ghost","Write title, caption and hashtags with AI");
  ui.ai.onclick=async()=>{
    if(!ui.topic.value.trim()){ui.msg.textContent="Type what the post is about first.";return}
    ui.ai.disabled=true;ui.msg.textContent="Writing…";
    try{const r=await api("POST","/api/optimize",{platform:"post",topic:ui.topic.value,hashtagCount:+ui.hc.value});const o=r.result||{};
      ui.title.value=(o.titles||[])[0]||ui.title.value;ui.cap.value=((o.description||"")+((o.hashtags||[]).length?"\n\n"+o.hashtags.join(" "):"")).trim();
      ui.ai.dataset.src=(r.sourceTags||[]).length?"1":"";
      drawViral(o,r.sourceTags||[]);drawPlats();
      ui.msg.textContent="Review and edit the text before publishing."}
    catch(e){ui.msg.textContent=e.message}ui.ai.disabled=false};
  ui.hc=h("select");for(const[v,t]of[[5,"5 hashtags"],[10,"10 hashtags (recommended mix)"],[15,"15 hashtags"]]){const o=h("option",null,t);o.value=v;if(v===10)o.selected=true;ui.hc.appendChild(o)}
  ui.win=h("button","bz-btn ghost","Find winning examples and write the script");ui.win.onclick=findWinners;ui.winres=h("div","win-res");
  ui.viral=h("div","viral-box");ui.chk=h("div","viral-chk");
  ui.cap.oninput=drawPlats;ui.title.oninput=drawPlats;
  ui.plats=h("div","chips");ui.tt=h("div","tt-panel");ui.tt.hidden=true;
  ui.when=h("input");ui.when.type="datetime-local";ui.when.onchange=drawPlats;
  ui.priv=h("select");for(const v of["private","unlisted","public"]){const o=h("option",null,v==="private"?"private (safest)":v);o.value=v;ui.priv.appendChild(o)}
  const lab=(t,el)=>{const l=h("label",null,t);l.appendChild(el);return l};
  const f=h("div","form");add(f,lab("Title",ui.title),lab("YouTube visibility",ui.priv),lab("Schedule (optional): ficx uploads at this time",ui.when));
  ui.go=h("button","bz-btn","Publish");ui.go.onclick=submit;
  const shareWA=async()=>{const t=((ui.title.value?ui.title.value+"\n\n":"")+ui.cap.value).trim();if(!t&&!file){ui.msg.textContent="Choose a file or write a caption first.";return false}
    try{if(file&&navigator.canShare&&navigator.canShare({files:[file]})){await navigator.share({files:[file],text:t});return true}}catch(e){if(e&&e.name==="AbortError")return false}
    window.open("https://wa.me/?text="+encodeURIComponent(t),"_blank","noopener");ui.msg.textContent="Your browser cannot attach the file for you. WhatsApp opened with the text: attach the video or photo in the group yourself.";return true};
  ui.wa=h("button","bz-btn ghost","Share to a WhatsApp group");ui.wa.style.marginLeft="8px";ui.wa.title="Opens your share window with the video or photo and caption. Pick WhatsApp, then the group.";ui.wa.onclick=shareWA;
  ui.waChk=document.createElement("input");ui.waChk.type="checkbox";const wl=h("label","pub-wa");wl.style.cssText="display:flex;gap:8px;align-items:center;margin:8px 0";add(wl,ui.waChk,h("span",null,"Also share to a WhatsApp group when I press Publish (not for scheduled posts: a person has to tap send in WhatsApp)"));
  ui.go.onclick=async()=>{if(ui.waChk.checked&&!ui.when.value)shareWA();submit()};
  ui.msg=h("p","bz-mut");ui.bar=h("div","pubbar");ui.barI=h("i");ui.bar.appendChild(ui.barI);ui.bar.hidden=true;ui.jobs=h("div","pub-jobs");ui.sched=h("div","pub-sched");
  add(w,ui.seg,ui.hint,lab("File",ui.file),ui.prev,f,lab("Caption / description",ui.cap),lab("Topic for AI (optional)",ui.topic),lab("How many hashtags",ui.hc),ui.ai,ui.win,ui.winres,ui.viral,ui.chk,h("h4",null,"Post to"),ui.plats,ui.tt,wl,ui.go,ui.wa,ui.bar,ui.msg,ui.jobs,ui.sched);
  drawType();
  setTimeout(()=>{refresh().catch(()=>{})},0); // also load connections right away, in case the tab was opened before this page finished loading
  return refresh;
}
export async function refresh(){
  // Each status is read on its own, with one retry, so one failing call can no longer lock every platform.
  const one=async path=>{for(let i=0;i<3;i++){try{return await api("GET",path)}catch(e){if(i===2)throw e;await sleep(1200)}}};
  const [g,s]=await Promise.allSettled([one("/api/google/status"),one("/api/social/status")]);
  const gv=g.status==="fulfilled"?g.value:null,sv=s.status==="fulfilled"?s.value:null,p=(sv&&sv.providers)||{};
  conn={youtube:!!(gv&&gv.connected),facebook:!!(p.facebook||{}).connected,instagram:!!(p.instagram||{}).connected,tiktok:!!(p.tiktok||{}).connected,telegram:!!(p.telegram||{}).connected};
  const bad=[];if(!gv)bad.push("Google: "+((g.reason&&g.reason.message)||"failed"));if(!sv)bad.push("Social: "+((s.reason&&s.reason.message)||"failed"));
  if(ui.msg)ui.msg.textContent=bad.length?"Could not read your connections ("+bad.join(" | ").slice(0,300)+"). Reload the page and open Publish again.":"";
  drawPlats();
  takeDraft();loadSched();
}
function takeDraft(){
  const d=window.__bzDraft;if(!d)return;window.__bzDraft=null;
  if(d.caption!=null&&d.caption!=="")ui.cap.value=d.caption;
  if(d.title!=null)ui.title.value=d.title;
  if(d.topic!=null)ui.topic.value=d.topic;
  if(d.schedule)ui.when.value=d.schedule;
  drawPlats();ui.msg.textContent="Filled in from your fix. Choose a file and platforms, then publish.";
}
function drawType(){
  const t=TYPES[type];
  ui.seg.querySelectorAll("button").forEach(b=>b.classList.toggle("on",b.dataset.t===type));
  ui.hint.textContent=t.hint;ui.file.accept=t.accept;
  plats=new Set(t.plats.filter(p=>conn[p]));
  drawPlats();
}
function drawPlats(){
  const t=TYPES[type],scheduled=!!ui.when.value;
  ui.plats.innerHTML="";
  for(const p of["youtube","facebook","instagram","tiktok","telegram"]){
    const okType=t.plats.includes(p),okSched=true,connected=!!conn[p];
    const v=verdict(p),usable=okType&&okSched&&connected&&!v.block;
    if(!usable)plats.delete(p);
    const b=h("button","chip"+(plats.has(p)?" on":"")+(usable?"":" off"),PN[p]+(!okType?" (not for this type)":!connected?" (connect first)":!okSched?" (can't schedule)":v.block?" - "+v.block:""));
    b.onclick=()=>{if(!usable)return;if(plats.has(p))plats.delete(p);else plats.add(p);drawPlats()};
    ui.plats.appendChild(b);
    if(plats.has(p)&&v.note)ui.plats.appendChild(h("p","bz-mut pub-note",PN[p]+": "+v.note));
  }
  drawTT();drawCheck();
}

/* ---------- Winning examples + script / design brief (real YouTube data) ---------- */
const fmtN=n=>n>=1e6?(n/1e6).toFixed(1)+"M":n>=1e3?(n/1e3).toFixed(1)+"K":String(n);
async function findWinners(){
  const topic=(ui.topic.value||ui.title.value||"").trim();
  if(!topic){ui.msg.textContent="Type what the post is about in the Topic box first.";return}
  ui.win.disabled=true;ui.winres.innerHTML="";ui.winres.appendChild(h("p","bz-mut","Finding what is working right now"+(type==="image"?" and studying the designs":"")+". This can take up to a minute for unusual topics…"));
  try{drawWinners(await api("POST","/api/viral/research",{topic,kind:type}),topic)}
  catch(e){ui.winres.innerHTML="";ui.winres.appendChild(h("p","bz-err",e.message))}
  ui.win.disabled=false;
}
function drawWinners(r,topic){
  const a=r.analysis||{},sc=a.script||{},dg=a.design||{},box=ui.winres;box.innerHTML="";
  box.appendChild(h("h4",null,type==="image"?"Top-performing designs in your niche ("+r.source+")":"Top-performing videos on your topic ("+r.source+")"));
  if(type==="image")box.appendChild(h("p","bz-mut pub-note","These are the cover images that earned the most views compared with their channel's size. Look at the layout, the text size and the colours."));
  if(r.note)box.appendChild(h("p","bz-mut pub-note",r.note));
  const grid=h("div","win-grid");
  for(const v of r.videos||[]){
    const card=h("div","win-card"),link=h("a");link.href=v.url;link.target="_blank";link.rel="noopener";
    const im=h("img");im.src=v.thumb;im.alt="";im.loading="lazy";im.referrerPolicy="no-referrer";link.appendChild(im);
    card.appendChild(link);
    const t=h("a","win-t",v.title);t.href=v.url;t.target="_blank";t.rel="noopener";card.appendChild(t);
    card.appendChild(h("p","bz-mut pub-note",v.channel+(v.subs?" · "+fmtN(v.subs)+" subscribers":"")));
    card.appendChild(h("p","pub-note",fmtN(v.views)+" views · "+fmtN(Math.round(v.perDay))+"/day"+(v.ratio>=2?" · "+v.ratio.toFixed(1)+"x its channel size":"")+(v.durSec?" · "+Math.round(v.durSec)+" s":"")));
    if(v.why)card.appendChild(h("p","bz-mut pub-note",v.why));
    grid.appendChild(card);
  }
  box.appendChild(grid);
  if((a.patterns||[]).length){box.appendChild(h("h4",null,"What the winners have in common"));const ul=h("ul","bz-mut");for(const p of a.patterns)ul.appendChild(h("li",null,p));box.appendChild(ul)}
  if((sc.beats||[]).length){
    let txt="HOOK: "+(sc.hook||"")+"\n";
    for(const b of sc.beats)txt+="\n["+(b.time||"")+"]\nSAY: "+(b.say||"")+"\nSHOW: "+(b.show||"")+(b.onscreen?"\nON SCREEN: "+b.onscreen:"")+"\n";
    if(sc.cta)txt+="\nCALL TO ACTION: "+sc.cta;
    box.appendChild(h("h4",null,"Your script ("+(sc.lengthSec?"about "+sc.lengthSec+" seconds":sc.format||"")+")"));
    box.appendChild(copyBox("Script (edit it, then record)",txt.trim(),true));
  }
  if(dg.layout){
    const slides=(dg.slides||[]).map((x,i)=>"SLIDE "+(i+1)+": "+(x.headline||"")+(x.body?"\n  TEXT: "+x.body:"")+(x.visual?"\n  VISUAL: "+x.visual:"")).join("\n");
    const txt=[(dg.format?"FORMAT: "+dg.format:""),(slides?slides:""),"LAYOUT: "+dg.layout,"TEXT ON IMAGE: "+(dg.text||""),"COLORS: "+(dg.colors||""),"ELEMENTS:\n- "+(dg.elements||[]).join("\n- "),"OTHER CONCEPTS:\n- "+(dg.variations||[]).join("\n- ")].filter(Boolean).join("\n\n");
    box.appendChild(h("h4",null,type==="image"?"Design brief":"Thumbnail concept"));box.appendChild(copyBox("Design brief",txt,true));
  }
  if((a.titles||[]).length){
    box.appendChild(h("h4",null,"Title options (click one to use it)"));const row=h("div","chips");
    for(const t of a.titles){const b=h("button","chip",t);b.onclick=()=>{ui.title.value=t;drawPlats()};row.appendChild(b)}box.appendChild(row);
  }
  const use=h("button","bz-btn","Use this title and caption in the form");
  use.onclick=()=>{
    if((a.titles||[])[0])ui.title.value=a.titles[0];
    const keep=(ui.cap.value.match(/#[\p{L}\p{N}_]+/gu)||[]).join(" "),capt=(sc.caption||sc.hook||"").trim();
    if(capt)ui.cap.value=(capt+(keep?"\n\n"+keep:"")).trim();
    drawPlats();ui.msg.textContent="Title and caption filled in. Record or design it, choose the file, then publish.";window.scrollTo({top:ui.title.getBoundingClientRect().top+scrollY-120,behavior:"smooth"});
  };
  box.appendChild(use);
  if(type==="image"){
    const q=encodeURIComponent(topic);
    box.appendChild(h("h4",null,"Find image and ad posts that are performing well"));
    const wrap=h("div","chips");
    for(const[n,u]of[["Facebook Ad Library (image ads)","https://www.facebook.com/ads/library/?active_status=active&ad_type=all&country=ALL&media_type=image&search_type=keyword_unordered&q="+q],["Pinterest","https://www.pinterest.com/search/pins/?q="+q],["Canva templates","https://www.canva.com/templates/?query="+q],["Behance","https://www.behance.net/search/projects?search="+q],["Google Images (past month)","https://www.google.com/search?tbm=isch&tbs=qdr:m&q="+q]]){
      const a2=h("a","chip",n);a2.href=u;a2.target="_blank";a2.rel="noopener";wrap.appendChild(a2)}
    box.appendChild(wrap);
    box.appendChild(h("p","bz-mut pub-note","Tip: in the Ad Library, an ad that has been running for 30 days or more is usually making money, because advertisers stop paying for ads that don't work. Open the ones with the oldest start date."));
  }
  const tags=(r.tags||[]).slice(0,3);
  if(tags.length){
    box.appendChild(h("h4",null,"See what is trending live on the other platforms"));
    const L=[["TikTok",t=>"https://www.tiktok.com/tag/"+encodeURIComponent(t)],["Instagram",t=>"https://www.instagram.com/explore/tags/"+encodeURIComponent(t)+"/"],["Facebook",t=>"https://www.facebook.com/hashtag/"+encodeURIComponent(t)],["YouTube",t=>"https://www.youtube.com/hashtag/"+encodeURIComponent(t)]];
    const wrap=h("div","chips");
    for(const[n,f]of L)for(const t of tags){const a2=h("a","chip","#"+t+" on "+n);a2.href=f(t);a2.target="_blank";a2.rel="noopener";wrap.appendChild(a2)}
    box.appendChild(wrap);
  }
  box.appendChild(h("p","bz-mut pub-note","Real performance numbers come from YouTube. TikTok, Instagram and Facebook don't share their top content by topic with apps, so use the links to see it live. Use the winners as a model for structure and style. Make your own version and don't copy their wording or visuals."));
}

/* ---------- Viral pack: hooks, real top-video tags, pre-post checklist ---------- */
function drawViral(o,src){
  ui.viral.innerHTML="";
  if((o.hooks||[]).length){
    ui.viral.appendChild(h("p","bz-mut","Hook options for the first line (click one to use it):"));
    const row=h("div","chips");
    for(const hk of o.hooks){const b=h("button","chip",hk);b.onclick=()=>{const rest=ui.cap.value.split("\n").slice(1).join("\n");ui.cap.value=(hk+"\n"+rest).trim();drawPlats()};row.appendChild(b)}
    ui.viral.appendChild(row);
  }
  const t=o.tagTiers||{},names={broad:"Broad",mid:"Mid-size",niche:"Niche",micro:"Micro",trending:"Trending"};
  const lines=Object.keys(names).filter(k=>(t[k]||[]).length).map(k=>names[k]+": "+t[k].join(" "));
  if(lines.length){ui.viral.appendChild(h("p","bz-mut","Hashtag mix (a pyramid of popular to very specific tags, so you can rank in small searches and still be found in big ones):"));for(const l of lines)ui.viral.appendChild(h("p","bz-mut pub-note",l))}
  if(src.length)ui.viral.appendChild(h("p","bz-mut pub-note","Based on tags used by the most-viewed recent YouTube videos on your topic: "+src.slice(0,12).join(" ")));
  else ui.viral.appendChild(h("p","bz-mut pub-note","Tags are AI-written. Add YOUTUBE_API_KEY in the admin console so they are based on real top-video tags."));
  for(const n of o.notes||[])ui.viral.appendChild(h("p","bz-mut pub-note","Test next: "+n));
}
function drawCheck(){
  if(!ui.chk)return;
  const txt=ui.cap.value,first=txt.split("\n")[0].replace(/#[\p{L}\p{N}_]+/gu,"").trim(),tags=txt.match(/#[\p{L}\p{N}_]+/gu)||[],body=txt.replace(/#[\p{L}\p{N}_]+/gu,"").trim();
  const items=[
    [first.length>=8&&first.length<=125,"Hook: the first line is short and grabs attention ("+first.length+" characters, aim for 125 or fewer)"],
    [tags.length>=3&&tags.length<=15,"Hashtags: "+tags.length+" (3 to 15 works; YouTube ignores all hashtags if you use more than 15)"],
    [/\b(comment|share|save|follow|subscribe|tell me|tag|link in bio|dm|vote|reply)\b/i.test(body),"Call to action: ask for a comment, share or save"],
    [body.split(/\s+/).filter(Boolean).length>=12,"Keywords: describe the video in plain words people would search"]
  ];
  if(meta&&type!=="image"){
    if(type==="short")items.push([meta.w<=meta.h,"Format: vertical or square for Shorts, Reels and TikTok"]);
    items.push([meta.d<=60||type!=="short","Length: "+Math.round(meta.d)+" s (short videos under about 60 s tend to hold viewers to the end)"]);
  }
  const ok=items.filter(i=>i[0]).length;
  ui.chk.innerHTML="";
  ui.chk.appendChild(h("p","bz-mut","Pre-post checklist: "+ok+" of "+items.length+". Rules of thumb from platform guidance. They improve your odds but can't guarantee reach."));
  for(const[pass,msg]of items)ui.chk.appendChild(h("p",pass?"bz-mut pub-note":"pub-note tt-warn",(pass?"✓ ":"• ")+msg));
}

/* ---------- TikTok options (required by TikTok's Content Posting API guidelines) ---------- */
const PRIV={PUBLIC_TO_EVERYONE:"Everyone",MUTUAL_FOLLOW_FRIENDS:"Friends",FOLLOWER_OF_CREATOR:"Followers",SELF_ONLY:"Only me"};
const MUSIC_URL="https://www.tiktok.com/legal/page/global/music-usage-confirmation/en",BC_URL="https://www.tiktok.com/legal/page/global/bc-policy/en";
function drawTT(){
  const show=plats.has("tiktok");ui.tt.hidden=!show;
  if(!show||ui.tt.dataset.ready)return;
  ui.tt.dataset.ready="1";ui.tt.innerHTML="";ui.tt.appendChild(h("p","bz-mut","Loading your TikTok account…"));
  api("GET","/api/tiktok/creator").then(c=>{ttInfo=c;buildTT(c)}).catch(e=>{
    ui.tt.dataset.ready="";tt=null;ui.tt.innerHTML="";ui.tt.appendChild(h("p","bz-mut","TikTok: "+e.message))});
}
function cb(label,get,set,disabled){
  const l=h("label","tt-cb"),i=h("input");i.type="checkbox";i.checked=!!get();i.disabled=!!disabled;
  i.onchange=()=>{set(i.checked);paintTT()};l.appendChild(i);l.appendChild(document.createTextNode(" "+label));return l;
}
function buildTT(c){
  tt={privacy:"",comment:false,duet:false,stitch:false,commercial:false,yourBrand:false,branded:false,consent:false};
  ui.tt.innerHTML="";
  const who=h("div","tt-who");
  if(c.avatar){const im=h("img");im.src=c.avatar;im.alt="";im.referrerPolicy="no-referrer";who.appendChild(im)}
  const wt=h("span");wt.appendChild(document.createTextNode("Posting to TikTok as "));wt.appendChild(h("b",null,c.nickname||c.username||"your account"));
  if(c.username)wt.appendChild(document.createTextNode(" (@"+c.username+")"));who.appendChild(wt);
  const sel=h("select"),ph=h("option",null,"Select who can view this video");ph.value="";sel.appendChild(ph);
  for(const o of c.options||[]){const op=h("option",null,PRIV[o]||o);op.value=o;sel.appendChild(op)}
  sel.onchange=()=>{tt.privacy=sel.value;paintTT()};
  const pl=h("label",null,"Who can view this video");pl.appendChild(sel);
  ui.ttNote=h("p","bz-mut pub-note");
  ui.ttInter=h("div","tt-row");
  ui.ttCom=h("div");ui.ttBrand=h("div");ui.ttConsent=h("div");
  add(ui.tt,who,pl,ui.ttNote,ui.ttInter,ui.ttCom,ui.ttBrand,ui.ttConsent);
  paintTT();
}
function paintTT(){
  if(!tt||!ttInfo)return;const c=ttInfo;
  ui.ttNote.textContent=c.audited?"":"Until TikTok approves ficx, every post is sent as private (Only me), whichever option you choose.";
  ui.ttInter.innerHTML="";ui.ttInter.appendChild(h("span","bz-mut","Allow users to:"));
  add(ui.ttInter,
    cb("Comment"+(c.comment_disabled?" (off in your TikTok settings)":""),()=>tt.comment,v=>tt.comment=v,c.comment_disabled),
    cb("Duet"+(c.duet_disabled?" (off in your TikTok settings)":""),()=>tt.duet,v=>tt.duet=v,c.duet_disabled),
    cb("Stitch"+(c.stitch_disabled?" (off in your TikTok settings)":""),()=>tt.stitch,v=>tt.stitch=v,c.stitch_disabled));
  ui.ttCom.innerHTML="";
  ui.ttCom.appendChild(cb("Disclose commercial content (this video promotes a brand, product or service)",()=>tt.commercial,v=>{tt.commercial=v;if(!v){tt.yourBrand=false;tt.branded=false}}));
  ui.ttBrand.innerHTML="";
  if(tt.commercial){
    add(ui.ttBrand,cb("Your brand: you promote yourself or your own business",()=>tt.yourBrand,v=>tt.yourBrand=v),
      cb("Branded content: you promote another brand or a third party",()=>tt.branded,v=>tt.branded=v));
    let m="Turn on at least one option to continue.";
    if(tt.yourBrand&&!tt.branded)m="Your video will be labeled “Promotional content”.";
    if(tt.branded)m="Your video will be labeled “Paid partnership”.";
    ui.ttBrand.appendChild(h("p","bz-mut pub-note",m));
    if(tt.branded&&(tt.privacy==="SELF_ONLY"||!c.audited))ui.ttBrand.appendChild(h("p","pub-note tt-warn","Branded content can't be private. Choose a public option once ficx is approved by TikTok."));
  }
  ui.ttConsent.innerHTML="";
  const l=h("label","tt-cb"),i=h("input");i.type="checkbox";i.checked=tt.consent;i.onchange=()=>{tt.consent=i.checked};l.appendChild(i);
  l.appendChild(document.createTextNode(" By posting, you agree to TikTok's "));
  const a=h("a",null,"Music Usage Confirmation");a.href=MUSIC_URL;a.target="_blank";a.rel="noopener";l.appendChild(a);
  if(tt.branded){l.appendChild(document.createTextNode(" and "));const b=h("a",null,"Branded Content Policy");b.href=BC_URL;b.target="_blank";b.rel="noopener";l.appendChild(b)}
  l.appendChild(document.createTextNode("."));ui.ttConsent.appendChild(l);
}
function ttCheck(){
  if(!tt)return"TikTok options are still loading. Wait a moment and try again.";
  if(!tt.privacy)return"TikTok: choose who can view this video.";
  if(tt.commercial&&!tt.yourBrand&&!tt.branded)return"TikTok: pick Your brand and/or Branded content, or turn commercial disclosure off.";
  if(tt.branded&&(tt.privacy==="SELF_ONLY"||!(ttInfo&&ttInfo.audited)))return"TikTok: Branded content can't be private (and posts stay private until TikTok approves ficx). Turn off Branded content.";
  if(!tt.consent)return"TikTok: tick the box to confirm TikTok's Music Usage Confirmation.";
  return"";
}
async function submit(){
  ui.msg.textContent="";
  if(!file){ui.msg.textContent="Choose a file first.";return}
  const list=[...plats];if(!list.length){ui.msg.textContent="Choose at least one platform that is connected.";return}
  if(file.size>300*1024*1024){ui.msg.textContent="The file is larger than 300 MB. Compress it first.";return}
  if(plats.has("tiktok")){const e=ttCheck();if(e){ui.msg.textContent=e;return}}
  const fd=new FormData();
  fd.append("type",type);fd.append("title",ui.title.value);fd.append("caption",ui.cap.value);fd.append("privacy",ui.priv.value);fd.append("platforms",list.join(","));
  if(plats.has("tiktok")){
    fd.append("tt_privacy",tt.privacy);fd.append("tt_consent",tt.consent?"1":"0");
    for(const[k,v]of[["tt_comment",tt.comment],["tt_duet",tt.duet],["tt_stitch",tt.stitch],["tt_commercial",tt.commercial],["tt_your_brand",tt.yourBrand],["tt_branded",tt.branded]])fd.append(k,v?"1":"0");
  }
  if(ui.when.value)fd.append("schedule",new Date(ui.when.value).toISOString());
  fd.append("file",file);
  ui.go.disabled=true;ui.bar.hidden=false;ui.barI.style.width="0";ui.jobs.innerHTML="";
  try{
    const tok=await auth.currentUser.getIdToken();
    const id=await new Promise((res,rej)=>{
      const x=new XMLHttpRequest();x.open("POST","/api/publish");x.setRequestHeader("Authorization","Bearer "+tok);
      x.upload.onprogress=e=>{if(e.lengthComputable){const p=Math.round(e.loaded/e.total*100);ui.barI.style.width=p+"%";ui.msg.textContent="Uploading to ficx… "+p+"%"}};
      x.onload=()=>{if(x.status<300){try{res(JSON.parse(x.responseText).id)}catch(e){rej(new Error("Unexpected reply"))}}else rej(new Error(x.responseText||("Upload failed ("+x.status+")")))};
      x.onerror=()=>rej(new Error("The connection dropped while uploading. Try again, or use a smaller file."));x.send(fd)});
    ui.msg.textContent="Publishing to each platform… keep this page open.";
    for(let i=0;i<900;i++){
      let d;try{d=await api("GET","/api/publish/status?id="+id)}catch(e){await sleep(4000);continue}
      render(d);if(d.done)break;await sleep(3000);
    }
    ui.msg.textContent="Finished.";loadSched();
  }catch(e){ui.msg.textContent=e.message}
  ui.go.disabled=false;
}
const SN={pending:"Waiting",running:"Uploading",done:"Posted",failed:"Failed"};
let schedTimer=null,pingTimer=null;
async function loadSched(){
  let list;try{list=await api("GET","/api/publish/scheduled")}catch(e){return}
  ui.sched.innerHTML="";
  clearTimeout(schedTimer);
  if(!pingTimer)pingTimer=setInterval(()=>{if(!document.hidden)fetch("/healthz").catch(()=>{})},4*60*1000); // keeps the free server awake while this page is open
  if(!list.length)return;
  if(list.some(x=>x.status==="pending"||x.status==="running"))schedTimer=setTimeout(()=>{if(ui.sched.isConnected)loadSched()},15000);
  ui.sched.appendChild(h("h4",null,"Scheduled posts (Instagram, TikTok and Telegram)"));
  for(const it of list){
    const row=h("div","pub-row"),st=h("span","bz-pill "+(it.status==="done"?"pass":it.status==="failed"?"fail":"warn"),(SN[it.status]||it.status).toUpperCase());
    const body=h("div");body.appendChild(h("b",null,(it.plats||[]).map(p=>PN[p]||p).join(" + ")+" · "+new Date(it.when).toLocaleString()));
    if(it.title)body.appendChild(h("p","bz-mut",it.title));
    if(it.status==="pending"&&new Date(it.when)<new Date())body.appendChild(h("p","bz-mut","The time has passed. The free server may have been asleep. It will send within a minute, or press Post now."));
    for(const r of it.results||[])body.appendChild(h("p","bz-mut",(PN[r.platform]||r.platform)+": "+r.state+(r.message?" - "+r.message:"")));
    if(it.status==="pending"){
      const n=h("button","bz-btn","Post now");n.onclick=async()=>{n.disabled=true;n.textContent="Sending...";try{await api("POST","/api/publish/scheduled/now",{id:it.id})}catch(e){ui.msg.textContent=e.message}setTimeout(loadSched,6000)};body.appendChild(n);
      const b=h("button","bz-btn ghost","Cancel");b.onclick=async()=>{b.disabled=true;try{await api("POST","/api/publish/scheduled/cancel",{id:it.id})}catch(e){ui.msg.textContent=e.message}loadSched()};body.appendChild(b)}
    add(row,st,body);ui.sched.appendChild(row);
  }
}
function render(d){
  ui.jobs.innerHTML="";
  for(const it of d.items){
    const row=h("div","pub-row"),st=h("span","bz-pill "+(it.state==="done"?"pass":it.state==="failed"?"fail":"warn"),it.state.toUpperCase());
    const body=h("div");body.appendChild(h("b",null,PN[it.platform]||it.platform));
    if(it.message)body.appendChild(h("p","bz-mut",it.message));
    if(it.url){const a=h("a",null,"Open post");a.href=it.url;a.target="_blank";a.rel="noopener";body.appendChild(a)}
    add(row,st,body);ui.jobs.appendChild(row);
  }
}
