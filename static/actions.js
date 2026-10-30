import{h,add,api,copy,copyBox}from"./ui.js";

const M=r=>{const m=Object.assign({},r.meta||{});m.domain=m.domain||(r.kind==="website"?r.target:"");m.name=m.name||m.channel||m.title||m.domain||r.target||"your brand";m.url=m.url||(m.domain?"https://"+m.domain+"/":"");return m};
const originOf=m=>{try{return new URL(m.url).origin}catch(e){return"https://"+(m.domain||"example.com")}};
const DAYS=["Sunday","Monday","Tuesday","Wednesday","Thursday","Friday","Saturday"];
const FALLBACK_IDEAS=["Beginner mistakes to avoid","Step-by-step how-to","Myth vs reality","Case study: before and after","Behind the scenes","Answer your audience's top 5 questions","Tools and setup I use","Weekly recap: what worked and what didn't"];

// ---------- ready-to-paste snippets (no AI needed) ----------
function snippets(key,r){
  const m=M(r),o=originOf(m),title=m.title||m.name,desc=m.description||("Learn more about "+m.name+"."),dom=m.domain||"example.com";
  switch(key){
  case"https":return{boxes:[
    {t:"Cloudflare (free): switch these on",x:"1. Cloudflare dashboard > SSL/TLS > Overview: choose Full.\n2. SSL/TLS > Edge Certificates: enable Always Use HTTPS and Automatic HTTPS Rewrites."},
    {t:"nginx redirect",x:"server {\n  listen 80;\n  server_name "+dom+";\n  return 301 https://$host$request_uri;\n}"},
    {t:".htaccess (Apache)",x:"RewriteEngine On\nRewriteCond %{HTTPS} off\nRewriteRule ^ https://%{HTTP_HOST}%{REQUEST_URI} [L,R=301]"}]};
  case"canonical":return{boxes:[{t:"Add inside <head>",x:'<link rel="canonical" href="'+(m.url||o+"/")+'">'}]};
  case"viewport":return{boxes:[{t:"Add inside <head>",x:'<meta name="viewport" content="width=device-width, initial-scale=1">'}]};
  case"lang":return{boxes:[{t:"Change your opening html tag",x:'<html lang="en">'}],note:"Use your page language code, e.g. am, ar, fr."};
  case"og":return{boxes:[{t:"Add inside <head> (social share previews)",x:['<meta property="og:type" content="website">','<meta property="og:title" content="'+title+'">','<meta property="og:description" content="'+desc+'">','<meta property="og:url" content="'+(m.url||o+"/")+'">','<meta property="og:image" content="'+o+'/og-image.jpg">','<meta name="twitter:card" content="summary_large_image">'].join("\n")}],note:"Create a 1200x630 image named og-image.jpg and upload it to your site root."};
  case"robots":return{boxes:[{t:"Save as "+o+"/robots.txt",x:"User-agent: *\nAllow: /\n\nUser-agent: GPTBot\nAllow: /\n\nUser-agent: ClaudeBot\nAllow: /\n\nUser-agent: PerplexityBot\nAllow: /\n\nUser-agent: Google-Extended\nAllow: /\n\nSitemap: "+o+"/sitemap.xml"}]};
  case"sitemap":return{boxes:[{t:"Save as "+o+"/sitemap.xml (add one <url> per page)",x:'<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n  <url>\n    <loc>'+(m.url||o+"/")+'</loc>\n    <lastmod>'+new Date().toISOString().slice(0,10)+'</lastmod>\n  </url>\n</urlset>'}],note:"Then submit it in Google Search Console > Sitemaps and Bing Webmaster Tools."};
  case"jsonld":{const j={"@context":"https://schema.org","@type":"Organization",name:m.name,url:m.url||o+"/",description:desc};return{boxes:[{t:"Paste inside <head> of your homepage",x:'<script type="application/ld+json">\n'+JSON.stringify(j,null,2)+"\n<\/script>"}],note:"Add logo and sameAs (your social profile links) for stronger results. Test at search.google.com/test/rich-results."}}
  case"llms":return{boxes:[{t:"Save as "+o+"/llms.txt",x:"# "+m.name+"\n> "+desc+"\n\n## Main pages\n- [Home]("+(m.url||o+"/")+"): "+title+"\n- [About]("+o+"/about): Who we are\n- [Contact]("+o+"/contact): How to reach us"}]};
  case"alt":return{boxes:[{t:"Pattern for every image",x:'<img src="photo.webp" alt="Describe what the image shows in one short sentence" width="800" height="600" loading="lazy">'}],note:"Describe the content, not 'image of'. Skip alt text (alt=\"\") only for purely decorative images."};
  case"noindex":return{boxes:[{t:"Remove this tag from the page <head>",x:'<meta name="robots" content="noindex">'},{t:"Also check",x:"- The X-Robots-Tag HTTP header\n- Your CMS setting 'Discourage search engines from indexing this site'\n- robots.txt rules that block this page"}]};
  case"perf":return{boxes:[
    {t:"Preload the main hero image",x:'<link rel="preload" as="image" href="/hero.webp" fetchpriority="high">'},
    {t:"Prevent layout shift and lazy-load other images",x:'<img src="/photo.webp" width="800" height="600" loading="lazy" decoding="async" alt="...">'},
    {t:"Do not block rendering with scripts",x:'<script src="/app.js" defer></script>'},
    {t:"Cloudflare (free) settings",x:"Speed > Optimization: enable Brotli and Early Hints.\nCaching > Configuration: Browser Cache TTL = 1 month.\nRules > Cache Rules: cache static files (css, js, images, fonts).\nConvert images to WebP/AVIF before upload (squoosh.app is free)."}]};
  case"review":return{boxes:[
    {t:"In-app rating prompt (show after a success moment)",x:"Enjoying "+m.name+"? A quick rating helps us a lot. It takes 10 seconds."},
    {t:"Reply to a negative review",x:"Thank you for the honest feedback. I'm sorry about your experience. We are working on a fix and will release it in the next update. If you email us the details at support@yourapp.com we will make sure it is solved."},
    {t:"Reply to a positive review",x:"Thank you so much! Reviews like yours help other people find "+m.name+". Tell us what you'd like to see next."}]};
  case"release":return{boxes:[{t:"Release notes template",x:"What's new\n- [Your best new feature, one line]\n- Faster and more stable\n- Fixes for issues you reported. Thank you for the feedback!"}]};
  case"engage":return{boxes:[
    {t:"Pinned comment option 1",x:"Which part was most useful for you? Tell me below and I'll reply to every comment."},
    {t:"Pinned comment option 2",x:"Quick question: what should the next video/post cover? Vote with a comment and I'll make the top one."},
    {t:"Spoken like-ask (say it right after you deliver the key value)",x:"If that tip saved you time, tap like so more people can find it, and tell me in the comments which step you'll try first."},
    {t:"End-of-video call to action",x:"If you want the next part, subscribe and turn on notifications. Next time I'll show you [next topic]."}]};
  }
  return{boxes:[]};
}
const snip=key=>(c,r)=>{const s=snippets(key,r);for(const x of s.boxes)c.appendChild(copyBox(x.t,x.x));if(s.note)c.appendChild(h("p","bz-mut",s.note))};

// ---------- AI-powered workspaces ----------
const opt=body=>api("POST","/api/optimize",body);
function loading(c,msg){c.querySelectorAll(".bz-live-msg").forEach(x=>x.remove());const p=h("p","bz-mut bz-live-msg",msg||"Writing your ready-to-use content…");c.appendChild(p);return p}
function failMsg(c,e,retry){const d=h("div");d.appendChild(h("p","bz-err",e.message||String(e)));if(retry){const b=h("button","bz-btn ghost","Try again");b.onclick=retry;d.appendChild(b)}c.appendChild(d)}
function options(c,label,items){if(!items||!items.length)return;const d=h("div");d.appendChild(h("h4",null,label));for(const t of items){const row=h("div","bz-opt"),b=h("button","bz-mini","Copy");b.onclick=()=>copy(t,b);add(row,h("span",null,t),b);d.appendChild(row)}c.appendChild(d)}
function compare(c,label,cur,sug,editable){const g=h("div","bz-two");g.appendChild(copyBox("Current "+label,cur||"(empty)"));const s=copyBox("Suggested "+label,sug||"",editable);g.appendChild(s);c.appendChild(g);return s}
const ai=(make,buildBody)=>async(c,r,item)=>{
  const run=async()=>{
    c.innerHTML="";const l=loading(c);
    try{const res=await opt(buildBody(r,item));l.remove();make(c,res.result||{},res.current||{},r,item)}
    catch(e){l.remove();failMsg(c,e,run)}
  };
  await run();
};

const videoFix=ai((c,o,cur,r,item)=>{
  const sTitle=compare(c,"title",cur.title,(o.titles||[])[0],true);
  const sDesc=compare(c,"description",cur.description,o.description,true);
  const sTags=compare(c,"tags",(cur.tags||[]).join(", "),(o.tags||[]).join(", "),true);
  if((o.hashtags||[]).length)c.appendChild(copyBox("Hashtags (put 3 at the end of the description)",o.hashtags.join(" ")));
  options(c,"More title options",(o.titles||[]).slice(1));options(c,"Opening hooks (first 10 seconds)",o.hooks);options(c,"Thumbnail ideas",o.ideas);
  const vid=(item&&item.videoId)||(r.meta||{}).weakId;
  if(vid){const st=h("p","bz-mut"),b=h("button","bz-btn","Apply to my YouTube video");
    b.onclick=async()=>{b.disabled=true;st.textContent="Updating video…";
      try{await api("POST","/api/youtube/apply",{videoId:vid,title:sTitle.ta.value,description:sDesc.ta.value,tags:sTags.ta.value.split(",").map(x=>x.trim()).filter(Boolean)});st.textContent="Done. The video now uses the new title, description and tags."}
      catch(e){st.textContent=e.message+" (Applying needs Connected Google and a video on your own channel.)"}b.disabled=false};
    add(c,h("p","bz-mut","Applying changes the live video. Review the text above first."),b,st)}
},(r,item)=>({platform:"youtube",videoId:(item&&item.videoId)||(r.meta||{}).weakId||(r.meta||{}).topId||"",topic:(r.meta||{}).niche||""}));

const seoFix=ai((c,o,cur,r)=>{
  const m=M(r);
  const t=compare(c,"page title",m.title,(o.titles||[])[0],true),d=compare(c,"meta description",m.description,o.description,true);
  const html=copyBox("Ready-to-paste HTML",'<title>'+t.ta.value+'</title>\n<meta name="description" content="'+d.ta.value.replace(/"/g,"&quot;")+'">\n<h1>'+((o.titles||[])[1]||t.ta.value)+'</h1>');
  c.appendChild(html);options(c,"More title / H1 options",(o.titles||[]).slice(1));
  c.appendChild(h("p","bz-mut","Edit the boxes above, then paste the snippet into your page's <head>. Keep the title under 60 characters and the description under 155."));
},(r)=>{const m=M(r);return{platform:"website",title:m.title,description:m.description,topic:m.name+" "+(m.domain||"")}});

const queriesFix=ai((c,o,cur,r)=>{
  options(c,"Title tag options for these queries",o.titles);
  c.appendChild(copyBox("Meta description to use",o.description||""));
  options(c,"Opening lines for the page / blog post",o.hooks);
  c.appendChild(h("p","bz-mut","Make (or update) one page per query group, put the main query in the title and first paragraph, then link to it from your homepage."));
},(r)=>({platform:"website",topic:"Search queries to rank for: "+((r.meta||{}).lowCtr||(r.meta||{}).queries||r.target),goal:"Win more clicks from Google"}));

const faqFix=ai((c,o,cur,r)=>{
  const q=o.titles||[],a=o.hooks||[],items=q.map((x,i)=>({q:x,a:a[i]||""})).filter(x=>x.a);
  for(const it of items)c.appendChild(copyBox("Q: "+it.q,it.a,true));
  const j={"@context":"https://schema.org","@type":"FAQPage",mainEntity:items.map(it=>({"@type":"Question",name:it.q,acceptedAnswer:{"@type":"Answer",text:it.a}}))};
  c.appendChild(copyBox("FAQ structured data (paste into your page <head>)",'<script type="application/ld+json">\n'+JSON.stringify(j,null,2)+"\n<\/script>"));
  c.appendChild(h("p","bz-mut","Publish these questions and answers visibly on a page too. Check the facts first: AI wrote the draft."));
},(r)=>{const m=M(r);return{platform:"faq",topic:m.name+": "+(m.description||m.domain)}});

const bioFix=ai((c,o,cur,r)=>{
  const m=M(r);
  compare(c,"bio",m.bio||"(not provided)",o.description,true);
  options(c,"Other bio options",o.titles);
},(r)=>{const m=M(r);return{platform:"bio",description:m.bio||"",topic:(m.platform||"")+" account @"+(m.handle||""),goal:m.goal||""}});

const hashFix=ai((c,o,cur,r)=>{
  if((o.hashtags||[]).length)c.appendChild(copyBox("Hashtags (copy under your caption)",o.hashtags.join(" ")));
  if((o.tags||[]).length)c.appendChild(copyBox("Keywords",o.tags.join(", ")));
  options(c,"Opening hooks + calls to action",o.hooks);
  c.appendChild(copyBox("Caption template",(o.titles||[])[0]?o.titles[0]+"\n\n"+(o.description||""):(o.description||""),true));
  options(c,"A/B testing tips",o.notes);
},(r)=>{const m=M(r);return{platform:m.platform||"instagram",topic:(m.goal||m.handle||r.target),description:m.bio||"",goal:m.goal||""}});

const appFix=ai((c,o,cur,r)=>{
  const m=M(r);
  compare(c,"title",m.name,(o.titles||[])[0],true);
  c.appendChild(copyBox("Store description (first 3 lines sell the benefit)",o.description||"",true));
  if((o.tags||[]).length)c.appendChild(copyBox("Keyword field (100 characters max)",o.tags.join(",").slice(0,100)));
  options(c,"More title options",(o.titles||[]).slice(1));options(c,"Screenshot caption ideas",o.ideas);
},(r)=>{const m=M(r);return{platform:"app",title:m.name,description:m.description||"",topic:m.name}});

const howto=ai((c,o)=>{
  c.appendChild(copyBox("Step-by-step implementation",o.description||"",true));
  options(c,"Ready-to-use examples",o.hooks);
},(r,item)=>({platform:"howto",topic:((item&&item.text)||"Improve results")+" (context: "+M(r).name+")"}));

// ---------- content calendar ----------
const pad=n=>String(n).padStart(2,"0");
const ymd=d=>d.getFullYear()+pad(d.getMonth()+1)+pad(d.getDate());
function ics(rows,name){
  const L=["BEGIN:VCALENDAR","VERSION:2.0","PRODID:-//Blitz AI//EN"];
  rows.forEach((x,i)=>{const e=new Date(x.date);e.setDate(e.getDate()+1);
    L.push("BEGIN:VEVENT","UID:blitz-"+ymd(x.date)+"-"+i+"@blitz.ai","DTSTAMP:"+new Date().toISOString().replace(/[-:]/g,"").slice(0,15)+"Z","DTSTART;VALUE=DATE:"+ymd(x.date),"DTEND;VALUE=DATE:"+ymd(e),"SUMMARY:"+("Post: "+x.text).replace(/[\n,;]/g," "),"END:VEVENT")});
  L.push("END:VCALENDAR");
  const a=document.createElement("a");a.href=URL.createObjectURL(new Blob([L.join("\r\n")],{type:"text/calendar"}));a.download=name+".ics";a.click();
}
const calendar=async(c,r)=>{
  const m=M(r),kind=r.kind;
  const cur=parseFloat(m.perWeek)||0;
  let per=kind==="youtube"?Math.max(2,Math.ceil(cur*1.3)):Math.max(3,Math.ceil(cur));per=Math.min(per,6);
  const topic=m.topTitle||m.niche||m.goal||m.description||m.name;
  c.innerHTML="";
  const ctl=h("div","form"),sel=h("input");sel.type="number";sel.min=1;sel.max=7;sel.value=per;
  const lab=h("label",null,"Posts per week");lab.appendChild(sel);ctl.appendChild(lab);c.appendChild(ctl);
  const out=h("div");c.appendChild(out);
  let ideas=null,hooks=[];
  const build=()=>{
    out.innerHTML="";const n=Math.max(1,Math.min(7,parseInt(sel.value)||per));
    const order=[2,4,6,1,3,5,0];let dows=order.slice(0,n);
    if(m.bestDay){const bi=DAYS.indexOf(m.bestDay);if(bi>=0&&!dows.includes(bi))dows[0]=bi}
    const start=new Date();start.setHours(12,0,0,0);start.setDate(start.getDate()+1);
    const rows=[];for(let i=0;i<28;i++){const d=new Date(start);d.setDate(start.getDate()+i);if(dows.includes(d.getDay()))rows.push({date:d,text:(ideas||FALLBACK_IDEAS)[rows.length%(ideas||FALLBACK_IDEAS).length]})}
    const list=h("div");
    rows.forEach((x,i)=>{
      const row=h("div","bz-cal"),inp=h("input");inp.value=x.text;inp.oninput=()=>{x.text=inp.value};
      const key="bz-cal:"+(r.target||"")+":"+ymd(x.date),chk=h("input");chk.type="checkbox";try{chk.checked=localStorage.getItem(key)==="1"}catch(e){}
      chk.onchange=()=>{try{localStorage.setItem(key,chk.checked?"1":"0")}catch(e){}};
      const b=h("button","bz-mini","Copy");b.onclick=()=>copy(x.date.toDateString()+": "+inp.value+(hooks[i%Math.max(hooks.length,1)]?"\nHook: "+hooks[i%hooks.length]:""),b);
      const lbl=h("span","bz-mut",x.date.toLocaleDateString(undefined,{weekday:"short",day:"numeric",month:"short"}));
      add(row,lbl,inp,chk,b);list.appendChild(row)});
    out.appendChild(list);
    const bar=h("div");bar.style.marginTop="12px";
    const cp=h("button","bz-btn ghost","Copy whole calendar");cp.onclick=()=>copy(rows.map(x=>x.date.toDateString()+" - "+x.text).join("\n"),cp);
    const dl=h("button","bz-btn","Download .ics (add to your phone calendar)");dl.style.marginLeft="8px";dl.onclick=()=>ics(rows,"blitz-content-calendar");
    add(bar,cp,dl);out.appendChild(bar);
    out.appendChild(h("p","bz-mut","Batch-create each week's content in one session, then schedule it (YouTube Studio, Meta Business Suite and TikTok all have free schedulers). Tick days as you publish."));
  };
  sel.onchange=build;
  const l=loading(c,"Generating content ideas for your calendar…");
  try{const res=await opt({platform:"ideas",topic:topic,goal:m.goal||""});ideas=(res.result.titles||[]).filter(Boolean);hooks=res.result.hooks||[];if(!ideas.length)ideas=null}catch(e){ideas=null;out.appendChild(h("p","bz-mut","AI ideas unavailable right now, so a proven topic mix is used. Edit any line."))}
  l.remove();build();
};

// ---------- routing: which workspace fits which problem ----------
export function fixFromCheck(ch,r){
  const n=ch.name,cat=ch.category||"";
  const T=[
   [/^HTTPS$/,snip("https")],[/^Title tag|^Meta description|^H1 heading/,seoFix],[/Canonical/,snip("canonical")],[/viewport/i,snip("viewport")],[/Language attribute/,snip("lang")],
   [/Open Graph|Twitter/,snip("og")],[/robots\.txt|AI crawlers/,snip("robots")],[/sitemap/i,snip("sitemap")],[/Structured data/,snip("jsonld")],[/llms\.txt/,snip("llms")],
   [/Mention rate/,faqFix],[/alt text/i,snip("alt")],[/Indexable/,snip("noindex")],
   [/Performance score|Largest Contentful|Layout shift|blocking time|Server response|HTML size|Lighthouse|Best-practices|Accessibility score/,snip("perf")]
  ];
  if(cat==="Search (Google)"&&/Click-through|position|clicks trend/i.test(n))return queriesFix;
  if(cat.startsWith("Traffic"))return howto;
  if(r.kind==="youtube"||cat==="YouTube"||cat==="Optimization"||cat==="Audience & reach"){
    if(/Typical views|Hit potential|Title length|Descriptions|tags|Titles with numbers/i.test(n))return(r.meta||{}).weakId?videoFix:howto;
  }
  if(cat==="Engagement"&&/Engagement rate|Like rate|Comment|Share rate/i.test(n))return snip("engage");
  if(/Upload frequency|Days since last upload|Posting frequency|Views trend|growth|Net subscriber|Reach rate|Engagement per view|Hit potential/i.test(n))return calendar;
  if(/Bio quality/.test(n))return bioFix;
  if(/Hashtag use|question or call to action/.test(n))return hashFix;
  if(r.kind==="app"){
    if(/Screenshots|Description length|Title length|Localization/.test(n))return appFix;
    if(/Average rating|Number of ratings|Current-version/.test(n))return snip("review");
    if(/Days since last update/.test(n))return snip("release");
  }
  for(const[re,fn]of T)if(re.test(n))return fn;
  return howto;
}
export function fixFromText(text,r){
  const t=text.toLowerCase();
  if(/schedul|calendar|batch|consisten|upload|publish|post(ing)? |frequency|cadence|weekly/.test(t))return calendar;
  if(/description|caption/.test(t)&&r.kind==="youtube"&&(r.meta||{}).weakId)return videoFix;
  if(/\bbio\b/.test(t))return bioFix;
  if(/hashtag|\btags?\b|keyword/.test(t))return r.kind==="youtube"&&(r.meta||{}).weakId?videoFix:hashFix;
  if(/title|thumbnail|headline|hook/.test(t))return r.kind==="youtube"&&(r.meta||{}).weakId?videoFix:r.kind==="website"?seoFix:hashFix;
  if(/comment|question|call to action|cta|engag/.test(t))return snip("engage");
  if(/meta|schema|structured|sitemap|robots/.test(t))return r.kind==="website"?seoFix:howto;
  return howto;
}
export function fixFromPlan(p,r){
  if(p.auto&&p.auto.startsWith("optimize-video:")){const id=p.auto.split(":")[1];return(c,rr)=>videoFix(c,rr,{videoId:id})}
  if(p.auto==="draft:meta"||p.auto==="draft:seo-content")return queriesFix;
  return fixFromText(p.title,r);
}
// toggle a workspace under an item; build once
export function mount(btn,box,builder,r,item){
  let built=false;
  btn.onclick=()=>{
    const open=!box.hidden;box.hidden=open;btn.textContent=open?"Open fix":"Close";
    if(!open&&!built){built=true;box.innerHTML="";Promise.resolve(builder(box,r,item)).catch(e=>failMsg(box,e))}
  };
}
