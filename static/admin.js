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
async function loadUsers(){
  const d=await api("GET","/api/admin/users"),box=$("users");box.innerHTML="";
  const U=d.users,pend=U.filter(u=>u.status==="pending").length;$("u-n").textContent="("+U.length+" accounts, "+pend+" waiting"+(d.approvalOn?"":", approval is OFF")+")";
  const t=h("table","crm-t");t.style.cssText="width:100%;border-collapse:collapse;font-size:13px";
  t.innerHTML="<thead><tr><th align=left>Email</th><th align=left>Name</th><th align=left>Via</th><th align=left>Signed up</th><th align=left>Last seen</th><th align=left>Status</th><th></th></tr></thead>";
  const order={pending:0,blocked:1,approved:2,admin:3};U.sort((a,b)=>order[a.status]-order[b.status]||b.created-a.created);
  const b=h("tbody");for(const u of U){const tr=h("tr");tr.style.borderTop="1px solid var(--line)";
    const dt=x=>x?new Date(x).toLocaleString():"never";
    for(const x of[u.email+(u.verified?"":" (unverified)"),u.name,(u.provider||"").replace(".com",""),dt(u.created),dt(u.lastLogin)])tr.appendChild(h("td",null,x)).style.padding="8px 6px";
    const st=h("td");st.style.padding="8px 6px";st.appendChild(h("span","bz-pill "+(u.status==="approved"||u.status==="admin"?"pass":u.status==="blocked"?"fail":"warn"),u.status));tr.appendChild(st);
    const ac=h("td");ac.style.cssText="padding:8px 6px;white-space:nowrap";
    const btn=(l,a)=>{const x=h("button","bz-mini",l);x.style.marginRight="4px";x.onclick=async()=>{if(a==="delete"&&!confirm("Delete this account permanently?"))return;if(a==="block"&&!confirm("Block "+u.email+"?"))return;try{await api("POST","/api/admin/users/set",{uid:u.uid,action:a});loadUsers()}catch(e){alert(e.message)}};ac.appendChild(x)};
    if(u.status!=="admin"){if(u.status!=="approved")btn("Approve","approve");if(u.status==="approved")btn("Revoke","revoke");if(u.status!=="blocked")btn("Block","block");btn("Delete","delete")}
    tr.appendChild(ac);b.appendChild(tr)}
  t.appendChild(b);const w=h("div");w.style.overflow="auto";w.appendChild(t);box.appendChild(w)}
onAuthStateChanged(auth,async u=>{
  if(!u){location.href="/app";return}
  try{await load();$("main").hidden=false;loadUsers().catch(()=>{})}catch(e){$("denied").hidden=false}
});
