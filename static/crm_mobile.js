// Phone layout for the CRM (screens up to 760 px wide):
// bottom tab bar with a centre Create button, a "More" sheet for everything else, a Create sheet, a search toggle,
// pull-to-refresh and an offline strip. Chats, lists, the pipeline and the record screen are adapted in crm_mobile.css.
export function mobile(X){
  const {h,icon,S,MENU,SECTIONS,nav,can,openRecord,ENT,label,refresh}=X;
  document.querySelectorAll(".mb-bar,.mb-ov,.mb-s,.mb-ptr,.fx-offline").forEach(n=>n.remove());
  const mq=matchMedia("(max-width:760px)");
  const avail=k=>MENU.some(m=>m[0]===k&&can(m[2],"v"));
  const pick=(...ks)=>ks.find(avail);
  const left=[pick("chats","tasks","dashboard"),pick("leads","contacts","dashboard")].filter((v,i,a)=>v&&a.indexOf(v)===i);
  const right=pick("deals","tasks","contacts","dashboard");
  const meta=k=>MENU.find(m=>m[0]===k);
  const bar=h("nav","mb-bar");bar.setAttribute("aria-label","Main");
  const tabKeys=[];
  const tab=(k,text,ic,fn)=>{const b=h("button","mb-tab");b.type="button";b.dataset.k=k;b.append(icon(ic,22),h("span",null,text));b.onclick=fn||(()=>nav(k));bar.appendChild(b);return b};
  for(const k of left){const m=meta(k);tabKeys.push(k);tab(k,k==="chats"?"Chats":m[1],m[3])}
  // centre: create
  const creatable=["leads","deals","contacts","companies","tasks"].filter(e=>can(e,"c"));
  if(creatable.length){const c=h("button","mb-plus");c.type="button";c.setAttribute("aria-label","Create");c.textContent="+";c.onclick=()=>sheet("Create new",body=>{const g=h("div","mb-list");for(const e of creatable){const b=h("button","mb-row",label(ENT[e].one));b.type="button";b.onclick=()=>{close();openRecord(e,null)};g.appendChild(b)}body.appendChild(g)});bar.appendChild(c)}
  if(right){const m=meta(right);tabKeys.push(right);tab(right,m[1],m[3])}
  const more=tab("more","More","grid",()=>sheet("Everything",moreBody));
  document.body.appendChild(bar);

  // bottom sheet
  let ov=null;
  function close(){if(ov){ov.classList.remove("in");const o=ov;ov=null;setTimeout(()=>o.remove(),180);document.body.classList.remove("mb-lock")}}
  function sheet(title,fill){close();ov=h("div","mb-ov");const sh=h("div","mb-sheet"),hd=h("div","mb-sh-h");const x=h("button","mb-sh-x","×");x.type="button";x.setAttribute("aria-label","Close");x.onclick=close;
    hd.append(h("b",null,title),x);const body=h("div","mb-sh-b");sh.append(h("i","mb-grab"),hd,body);ov.appendChild(sh);ov.onclick=e=>{if(e.target===ov)close()};
    fill(body);document.body.append(ov);document.body.classList.add("mb-lock");requestAnimationFrame(()=>ov&&ov.classList.add("in"))}
  function moreBody(body){
    for(const sec of SECTIONS){const its=sec.items.filter(i=>can(i[2],"v"));if(!its.length)continue;
      body.appendChild(h("h4","mb-sec",sec.t));const g=h("div","mb-grid");
      for(const[k,l,,ic]of its){const b=h("button","mb-cell"+(S.view===k?" on":""));b.type="button";b.append(icon(ic,22),h("span",null,l));b.onclick=()=>{close();nav(k)};g.appendChild(b)}
      body.appendChild(g)}
    const t=h("div","mb-theme");t.appendChild(h("span",null,"Theme"));
    for(const[k,l]of[["dark","Dark"],["slate","Slate"],["light","Light"]]){const b=h("button","mb-chipb",l);b.type="button";b.onclick=()=>{const o=document.querySelector('.bz-theme button[data-t="'+k+'"]');if(o)o.click()};t.appendChild(b)}
    body.appendChild(t)}

  // search toggle
  const top=document.querySelector(".bx-top"),sp=top&&top.querySelector(".sp");
  if(top&&sp){const s=h("button","mb-s");s.type="button";s.setAttribute("aria-label","Search");s.appendChild(icon("search",20));
    s.onclick=()=>{top.classList.toggle("msearching");if(top.classList.contains("msearching")){const i=document.getElementById("gsearch");i&&i.focus()}};
    top.insertBefore(s,sp.nextSibling);
    document.addEventListener("click",e=>{if(top.classList.contains("msearching")&&!e.target.closest(".bx-search,.mb-s"))top.classList.remove("msearching")})}

  // pull to refresh
  const ind=h("div","mb-ptr","Release to refresh");document.body.appendChild(ind);
  let y0=0,pull=0,on=false;
  addEventListener("touchstart",e=>{on=mq.matches&&scrollY<=0&&e.touches.length===1&&!ov&&S.view!=="chats"&&!String(S.view).startsWith("an:")&&document.getElementById("slider").hidden&&document.getElementById("modal").hidden;y0=e.touches[0].clientY;pull=0},{passive:true});
  addEventListener("touchmove",e=>{if(!on)return;pull=e.touches[0].clientY-y0;ind.classList.toggle("show",pull>80)},{passive:true});
  addEventListener("touchend",()=>{if(on&&pull>110){S.data={};refresh()}ind.classList.remove("show");on=false;pull=0});

  // offline strip
  const off=h("div","fx-offline","You are offline. Changes will not be saved until you reconnect.");off.hidden=navigator.onLine;document.body.appendChild(off);
  addEventListener("offline",()=>off.hidden=false);addEventListener("online",()=>off.hidden=true);

  return{sync(k){bar.querySelectorAll(".mb-tab").forEach(b=>b.classList.toggle("on",b.dataset.k===k||(b===more&&!tabKeys.includes(k))));}};
}
