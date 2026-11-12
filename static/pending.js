import{initializeApp}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-app.js";
import{getAuth,onAuthStateChanged,signOut}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-auth.js";
const auth=getAuth(initializeApp(window.FB)),$=i=>document.getElementById(i);
$("out").onclick=async()=>{await signOut(auth);location.href="/app"};
async function check(u){try{const t=await u.getIdToken(true);const r=await fetch("/api/approval",{headers:{Authorization:"Bearer "+t}});const d=await r.json();$("who").textContent=d.email?" ("+d.email+")":"";
 if(d.approved){location.href="/dashboard";return}$("st").textContent="Still waiting. Last checked "+new Date().toLocaleTimeString()}catch(e){$("st").textContent="Could not check right now."}}
onAuthStateChanged(auth,u=>{if(!u){location.href="/app";return}check(u);setInterval(()=>check(u),15000)});
