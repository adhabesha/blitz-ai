import{initializeApp}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-app.js";
import{getAuth,onAuthStateChanged,createUserWithEmailAndPassword,signInWithEmailAndPassword,updateProfile,sendPasswordResetEmail,sendEmailVerification,GoogleAuthProvider,signInWithPopup}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-auth.js";
const $=i=>document.getElementById(i);
const auth=getAuth(initializeApp(window.FB));
const native=/BlitzDesktop|BlitzMobile/.test(navigator.userAgent);
let mode="in",ob=null;
if(native)$("g-box").hidden=true;

function setMode(m){
  mode=m;$("t-in").classList.toggle("on",m==="in");$("t-up").classList.toggle("on",m==="up");
  $("f-name").hidden=m==="in";$("a-go").textContent=m==="in"?"Sign in":"Create account";
  $("a-title").textContent=m==="in"?"Welcome back":"Create your account";
  $("a-sub").textContent=m==="in"?"Sign in to measure and grow your website, channel and social accounts.":"Free to start. Takes less than a minute.";
  $("i-pass").autocomplete=m==="in"?"current-password":"new-password";msg("");
}
function msg(t,ok){const e=$("a-msg");e.textContent=t;e.className="auth-msg"+(ok?" ok":"")}
const FRIENDLY={"auth/invalid-credential":"Wrong email or password.","auth/wrong-password":"Wrong email or password.","auth/user-not-found":"No account with that email. Create one instead.","auth/email-already-in-use":"That email already has an account. Sign in instead.","auth/weak-password":"Password must be at least 6 characters.","auth/invalid-email":"That email address doesn't look right.","auth/too-many-requests":"Too many attempts. Wait a minute and try again.","auth/operation-not-allowed":"Email sign-in isn't switched on yet. Owner: enable Email/Password in Firebase > Authentication > Sign-in method.","auth/network-request-failed":"No connection. Check your internet and try again.","auth/popup-closed-by-user":"Sign-in was cancelled."};
const fail=e=>msg(FRIENDLY[e.code]||e.message||"Something went wrong.");
$("t-in").onclick=()=>setMode("in");$("t-up").onclick=()=>setMode("up");

$("a-go").onclick=async()=>{
  const email=$("i-email").value.trim(),pass=$("i-pass").value;
  if(!email||!pass){msg("Enter your email and password.");return}
  $("a-go").disabled=true;msg("");
  try{
    if(mode==="up"){
      const c=await createUserWithEmailAndPassword(auth,email,pass);
      const n=$("i-name").value.trim();if(n)await updateProfile(c.user,{displayName:n});
      try{await sendEmailVerification(c.user)}catch(e){}
    }else await signInWithEmailAndPassword(auth,email,pass);
  }catch(e){fail(e)}
  $("a-go").disabled=false;
};
$("a-google").onclick=async()=>{try{await signInWithPopup(auth,new GoogleAuthProvider())}catch(e){fail(e)}};
$("a-forgot").onclick=async()=>{
  const email=$("i-email").value.trim();if(!email){msg("Type your email above first.");return}
  try{await sendPasswordResetEmail(auth,email);msg("Password reset email sent. Check your inbox.",true)}catch(e){fail(e)}
};
for(const id of["i-email","i-pass","i-name"])$(id).addEventListener("keydown",e=>{if(e.key==="Enter")$("a-go").click()});

async function call(path,method,body){
  const t=await auth.currentUser.getIdToken();
  const r=await fetch(path,{method:method||"GET",headers:{Authorization:"Bearer "+t,"Content-Type":"application/json"},body:body?JSON.stringify(body):undefined});
  if(!r.ok)throw new Error(await r.text());return r.json();
}
const TABS={website:"website",youtube:"youtube",instagram:"socialc",tiktok:"socialc",facebook:"socialc",app:"app"};
const GOALS=[["website","\u{1F310} My website"],["youtube","\u25B6\uFE0F YouTube channel"],["instagram","\u{1F4F8} Instagram"],["tiktok","\u{1F3B5} TikTok"],["facebook","\u{1F465} Facebook page"],["app","\u{1F4F1} Mobile app"]];
const ROLES=["Creator","Business owner","Marketer / agency","Developer"];
function go(goals){const t=goals&&goals.length?TABS[goals[0]]:"";location.href="/dashboard"+(t?"?tab="+t:"")}

function startOnboarding(me,user){
  ob={step:0,goals:[],role:""};$("card-auth").hidden=true;$("card-ob").hidden=false;
  const name=(user.displayName||"").split(" ")[0]||"there";
  const steps=[
    ()=>`<div class="ob-emoji">\u26A1</div><h1>Hi ${esc(name)}, welcome to Blitz AI</h1><p>Your growth dashboard for websites, YouTube and social media.</p>
      <div class="ob-item"><div>\u{1F4CA}</div><div><b>Measure</b><span>Real KPIs, charts and forecasts from your data.</span></div></div>
      <div class="ob-item"><div>\u{1F6E0}\uFE0F</div><div><b>Fix</b><span>Every problem comes with ready-to-copy text, code or steps.</span></div></div>
      <div class="ob-item"><div>\u{1F4C5}</div><div><b>Plan</b><span>A 30-day plan and content calendar you can tick off.</span></div></div>`,
    ()=>`<h1>What do you want to grow?</h1><p>Pick all that apply. We'll open the right tool first.</p><div class="chips" id="goals"></div>
      <p style="margin-top:14px">I am a...</p><div class="chips" id="roles"></div>`,
    ()=>`<div class="ob-emoji">\u{1F517}</div><h1>Connect your accounts</h1><p>In the dashboard you can connect Google (Search Console, Analytics, YouTube) and Instagram, Facebook or TikTok so Blitz AI reads your real numbers.</p>
      <div class="ob-item"><div>\u{1F512}</div><div><b>You stay in control</b><span>You approve access on the platform's own screen and can disconnect any time.</span></div></div>
      <div class="ob-item"><div>\u2705</div><div><b>Or just paste a link</b><span>A website address or YouTube channel link works with no connection at all.</span></div></div>`
  ];
  const draw=()=>{
    $("ob-body").innerHTML=steps[ob.step]();
    $("ob-dots").innerHTML=steps.map((_,i)=>'<i class="'+(i===ob.step?"on":"")+'"></i>').join("");
    $("ob-back").style.visibility=ob.step?"visible":"hidden";$("ob-next").textContent=ob.step===steps.length-1?"Open my dashboard":"Next";
    if(ob.step===1){
      const g=$("goals"),r=$("roles");
      for(const[k,l]of GOALS){const b=document.createElement("button");b.className="chip"+(ob.goals.includes(k)?" on":"");b.textContent=l;b.onclick=()=>{ob.goals=ob.goals.includes(k)?ob.goals.filter(x=>x!==k):[...ob.goals,k];b.classList.toggle("on")};g.appendChild(b)}
      for(const l of ROLES){const b=document.createElement("button");b.className="chip"+(ob.role===l?" on":"");b.textContent=l;b.onclick=()=>{ob.role=l;r.querySelectorAll(".chip").forEach(x=>x.classList.toggle("on",x===b))};r.appendChild(b)}
    }
  };
  const finish=async()=>{try{await call("/api/profile","POST",{onboarded:true,goals:ob.goals,role:ob.role})}catch(e){}go(ob.goals)};
  $("ob-next").onclick=()=>{if(ob.step===steps.length-1)finish();else{ob.step++;draw()}};
  $("ob-back").onclick=()=>{if(ob.step>0){ob.step--;draw()}};
  $("ob-skip").onclick=finish;draw();
}
const esc=s=>s.replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));

onAuthStateChanged(auth,async u=>{
  if(!u)return;
  try{const me=await call("/api/me");if(me.onboarded)go(me.goals);else startOnboarding(me,u)}
  catch(e){go([])}
});
