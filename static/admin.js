import{initializeApp}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-app.js";
import{getAuth,onAuthStateChanged}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-auth.js";
import{$,h,add,setAuth,api,copyBox}from"./ui.js";
const auth=getAuth(initializeApp(window.FB));setAuth(auth);
const SRC={admin:["Set in admin","pass"],render:["Set in Render","warn"],none:["Missing","fail"]};

async function load(){
  const d=await api("GET","/api/admin/config");
  const set=d.items.filter(i=>i.source!=="none").length;
  $("prog").style.width=Math.round(set/d.items.length*100)+"%";$("prog-t").textContent=set+" of "+d.items.length+" settings configured";
  const rd=$("redirs");rd.innerHTML="";for(const[k,v]of Object.entries(d.redirects))rd.appendChild(copyBox(k[0].toUpperCase()+k.slice(1),v));
  const g=$("groups");g.innerHTML="";const groups={};for(const i of d.items)(groups[i.group]=groups[i.group]||[]).push(i);
  for(const[name,items]of Object.entries(groups)){
    const card=h("div","bz-card");card.style.marginTop="16px";card.appendChild(h("h3",null,name));
    const status=h("p","bz-mut");
    for(const it of items){
      const row=h("div","adm-row"),l=h("div"),p=h("div"),a=h("div","adm-in");
      const[t,cls]=SRC[it.source];
      add(l,h("b",null,it.label),h("small",null,it.help));
      add(p,h("span","bz-pill "+cls,t),h("span","bz-mut",it.preview?" "+it.preview:""));
      const inp=h("input");inp.type=it.secret?"password":"text";inp.placeholder=it.source==="none"?"Paste value":"New value";inp.autocomplete="off";
      const save=h("button","bz-mini","Save"),clr=h("button","bz-mini","Clear");
      save.onclick=async()=>{if(!inp.value.trim())return;save.textContent="Saving…";try{await api("POST","/api/admin/config",{key:it.key,value:inp.value});await load()}catch(e){alert(e.message);save.textContent="Save"}};
      clr.onclick=async()=>{if(!confirm("Remove the admin-set value for "+it.label+"?"))return;await api("POST","/api/admin/config",{key:it.key,value:""});await load()};
      add(a,inp,save);if(it.source==="admin")a.appendChild(clr);
      add(row,l,p,a);card.appendChild(row);
      if(it.test){const tb=h("button","bz-mini","Test "+name);tb.style.marginTop="8px";
        tb.onclick=async()=>{tb.textContent="Testing…";try{const r=await api("POST","/api/admin/test",{target:it.test});status.textContent=(r.ok?"\u2714 ":"\u2718 ")+r.message;status.className=r.ok?"bz-mut":"bz-err"}catch(e){status.textContent=e.message;status.className="bz-err"}tb.textContent="Test "+name};
        card.appendChild(tb)}
    }
    card.appendChild(status);g.appendChild(card);
  }
}
onAuthStateChanged(auth,async u=>{
  if(!u){location.href="/app";return}
  try{await load();$("main").hidden=false}catch(e){$("denied").hidden=false}
});
