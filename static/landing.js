(()=>{
const $=s=>document.querySelector(s);
let grade=null;
// reveal on scroll
try{const io=new IntersectionObserver(es=>es.forEach(e=>{if(e.isIntersecting){e.target.classList.add("in");io.unobserve(e.target)}}),{threshold:.12});
 document.querySelectorAll(".pain,.steps li,.feat,.plan,.gres,details").forEach(el=>{el.classList.add("rv");io.observe(el)})}catch(e){document.querySelectorAll(".rv").forEach(el=>el.classList.add("in"))}
// sticky mobile button: show after the hero, hide at the form
const st=$("#sticky"),demo=$("#demo");
if(st&&"IntersectionObserver"in window){let past=false,atForm=false;const upd=()=>st.classList.toggle("show",past&&!atForm);
 new IntersectionObserver(e=>{past=!e[0].isIntersecting&&e[0].boundingClientRect.top<0;upd()}).observe($(".hero"));
 new IntersectionObserver(e=>{atForm=e[0].isIntersecting;upd()}).observe(demo)}
const esc=s=>String(s).replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
const msg=(el,t,err)=>{el.textContent=t||"";el.classList.toggle("err",!!err)};
// ---- free website check
$("#gform").addEventListener("submit",async ev=>{
 ev.preventDefault();const url=$("#gurl").value.trim(),m=$("#gmsg"),res=$("#gres"),b=$("#gbtn");
 if(!url){msg(m,"Enter your website address, for example yourcompany.com",1);return}
 b.disabled=true;b.textContent="Checking...";msg(m,"Reading your site. This takes about 20 seconds.");res.hidden=true;
 try{
  const r=await fetch("/api/public/grade",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({url})});
  if(!r.ok)throw new Error((await r.text()).trim()||"Something went wrong");
  const g=await r.json();grade=g;msg(m,"");renderGrade(g,res);
 }catch(e){msg(m,e.message,1)}
 b.disabled=false;b.textContent="Check my website"});
function renderGrade(g,res){
 const c=g.score>=80?"#34d399":g.score>=55?"#fbbf24":"#f87171";
 const issues=(g.issues||[]).map(i=>`<div class="issue"><i class="dot ${esc(i.status)}"></i><div><strong>${esc(i.name)}</strong><span>${esc(i.value)}</span>${i.fix?`<div class="fixline"><b>How to fix:</b> ${esc(i.fix)}</div>`:""}</div></div>`).join("");
 const more=g.hidden>0?`<div class="locked"><p><b>${g.hidden} more problem${g.hidden>1?"s":""}</b> and the exact fix for every one are in your free report.</p><div class="cta-row" style="margin-top:0"><a class="btn" href="/app" data-cta="grade-signup">Get the full report free</a><a class="btn ghost" href="#demo" id="gdemo" data-cta="grade-demo">Book a demo</a></div></div>`
  :`<div class="locked"><p>Want ficx to handle the rest, from leads to posting to deals?</p><div class="cta-row" style="margin-top:0"><a class="btn" href="#demo" id="gdemo" data-cta="grade-demo">Book a demo</a></div></div>`;
 res.innerHTML=`<div><div class="ring" style="--p:${g.score};--c:${c}"><div><span><b>${g.score}</b><small>out of 100</small></span></div></div></div>
 <div><h3 style="margin-bottom:4px">${esc(g.domain)}</h3><div class="counts"><span><b>${g.pass}</b> passed</span><span><b>${g.warn}</b> to improve</span><span><b>${g.fail}</b> failing</span></div>${issues}${more}</div>`;
 res.hidden=false;res.classList.add("in");
 const gd=$("#gdemo");if(gd)gd.addEventListener("click",()=>{const f=$("#dform");if(f&&!f.company.value)f.company.value=g.domain});
 res.scrollIntoView({behavior:"smooth",block:"nearest"});
}
// ---- demo request
let cfg={};fetch("/api/public/config").then(r=>r.json()).then(x=>cfg=x||{}).catch(()=>{});
$("#dform").addEventListener("submit",async ev=>{
 ev.preventDefault();const f=ev.target,m=$("#dmsg"),b=$("#dbtn");
 if(f.name.value.trim().length<2){msg(m,"Please enter your name.",1);f.name.focus();return}
 if(!/^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/.test(f.email.value.trim())){msg(m,"Please enter a valid work email.",1);f.email.focus();return}
 b.disabled=true;b.textContent="Sending...";msg(m,"");
 const body={name:f.name.value,email:f.email.value,company:f.company.value,website:(grade&&grade.domain)||f.company.value,phone:f.phone.value,focus:f.focus.value,hp:f.hp.value,score:grade?grade.score:0};
 try{
  const r=await fetch("/api/public/demo",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
  if(!r.ok)throw new Error((await r.text()).trim()||"Something went wrong");
  const x=await r.json();f.hidden=true;$("#dok").hidden=false;
  const a=$("#dok-a");a.innerHTML="";
  if(x.booking){$("#dok-t").textContent="Pick a time that suits you now, or wait for our message.";a.innerHTML=`<a class="btn" href="${esc(x.booking)}" target="_blank" rel="noopener">Pick a time now</a>`}
  else $("#dok-t").textContent="We will message you shortly to agree a time for a 20-minute walkthrough. A confirmation is on its way to your inbox.";
  if(x.whatsapp)a.insertAdjacentHTML("beforeend",`<a class="btn ghost" href="https://wa.me/${esc(x.whatsapp)}?text=${encodeURIComponent("Hi, I just requested a ficx demo.")}" target="_blank" rel="noopener">Chat on WhatsApp</a>`);
 }catch(e){msg(m,e.message,1);b.disabled=false;b.textContent="Request my demo"}});
})();
