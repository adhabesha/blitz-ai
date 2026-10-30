export const $=i=>document.getElementById(i);
export const h=(t,c,x)=>{const e=document.createElement(t);if(c)e.className=c;if(x!=null)e.textContent=x;return e};
export const add=(p,...k)=>{for(const x of k)if(x)p.appendChild(x);return p};
const ICONS={
 globe:'<circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>',
 play:'<polygon points="5 3 19 12 5 21 5 3"/>',
 link:'<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>',
 users:'<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>',
 phone:'<rect x="5" y="2" width="14" height="20" rx="2" ry="2"/><line x1="12" y1="18" x2="12.01" y2="18"/>',
share:'<circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/>',
 zap:'<polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>'
};
export function icon(n,size=18){const s=document.createElementNS("http://www.w3.org/2000/svg","svg");for(const[k,v]of Object.entries({viewBox:"0 0 24 24",width:size,height:size,fill:"none",stroke:"currentColor","stroke-width":2,"stroke-linecap":"round","stroke-linejoin":"round"}))s.setAttribute(k,v);s.innerHTML=ICONS[n]||"";return s}
let authRef;export const setAuth=a=>{authRef=a};
export async function api(m,path,b){
  const tok=await authRef.currentUser.getIdToken();
  const r=await fetch(path,{method:m,headers:{Authorization:"Bearer "+tok,"Content-Type":"application/json"},body:b?JSON.stringify(b):undefined});
  if(!r.ok)throw new Error(await r.text());
  return r.json();
}
export function copy(text,btn){
  const done=()=>{if(btn){const o=btn.dataset.l||btn.textContent;btn.dataset.l=o;btn.textContent="Copied \u2713";setTimeout(()=>{btn.textContent=o},1500)}};
  const fb=()=>{const t=document.createElement("textarea");t.value=text;document.body.appendChild(t);t.select();try{document.execCommand("copy")}catch(e){}t.remove();done()};
  if(navigator.clipboard&&navigator.clipboard.writeText)navigator.clipboard.writeText(text).then(done,fb);else fb();
}
// titled textbox with a Copy button
export function copyBox(title,text,editable){
  const w=h("div","bz-copy"),hd=h("div","bz-copy-h"),ta=document.createElement("textarea");
  ta.value=text||"";ta.readOnly=!editable;ta.rows=Math.min(Math.max((text||"").split("\n").length,2),12);
  const b=h("button","bz-mini","Copy");b.onclick=()=>copy(ta.value,b);
  add(hd,h("span",null,title),b);add(w,hd,ta);w.ta=ta;return w;
}
