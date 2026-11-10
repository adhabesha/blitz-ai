import{h,add,api}from"./ui.js";

// A button + status line that runs an async apply action.
export function applyBtn(label,run,opts){
  opts=opts||{};
  const wrap=h("div","apply-row"),b=h("button","bz-btn",label),st=h("p","bz-mut apply-st");
  b.onclick=async()=>{
    if(opts.confirm&&!window.confirm(opts.confirm))return;
    b.disabled=true;st.textContent="Applying…";st.className="bz-mut apply-st";
    try{const r=await run();st.textContent=(r&&r.message)||"Done.";st.className="apply-st apply-ok";
      if(r&&r.url){const a=h("a",null," View the change");a.href=r.url;a.target="_blank";a.rel="noopener";st.appendChild(a)}}
    catch(e){st.textContent=e.message;st.className="bz-err apply-st"}
    b.disabled=false;
  };
  add(wrap,b,st);return wrap;
}

// Shown when a fix can't be applied automatically, so the person knows why.
export function noApply(c,reason){c.appendChild(h("p","bz-mut apply-no","Can't be applied automatically: "+reason))}

const bareHost=s=>String(s||"").toLowerCase().replace(/^https?:\/\//,"").split("/")[0].replace(/^www\./,"");
let siteCache=null;
export const siteStatus=async(force)=>{if(!siteCache||force)siteCache=await api("GET","/api/site/status");return siteCache};

// Setup form: connect the GitHub repository that deploys the website.
function setupCard(domain,done){
  const card=h("div","apply-setup");
  card.appendChild(h("h4",null,"Connect your website to apply fixes directly"));
  card.appendChild(h("p","bz-mut","Blitz AI changes your site by committing to the GitHub repository that your host (GitHub Pages, Netlify, Vercel, Cloudflare Pages) deploys from. Every change is one commit you can revert."));
  const steps=h("ol","bz-mut");
  for(const t of["GitHub > Settings > Developer settings > Personal access tokens > Fine-grained tokens > Generate new token.","Repository access: only select repositories, pick your website repo.","Permissions > Repository permissions > Contents: Read and write. Generate and copy the token."])steps.appendChild(h("li",null,t));
  card.appendChild(steps);
  const f=h("div","form"),field=(l,ph,type,val)=>{const lab=h("label",null,l),i=h("input");i.placeholder=ph;i.type=type||"text";if(val)i.value=val;lab.appendChild(i);f.appendChild(lab);return i};
  const dom=field("Your website address","mysite.com","text",domain||""),repo=field("GitHub repository","owner/name"),br=field("Branch","main","text","main"),pg=field("Homepage file in the repo","index.html","text","index.html"),tk=field("GitHub token","github_pat_…","password");
  const st=h("p","bz-mut"),b=h("button","bz-btn","Save and check connection");
  b.onclick=async()=>{b.disabled=true;st.textContent="Checking…";st.className="bz-mut";
    try{await api("POST","/api/site/connect",{domain:dom.value,repo:repo.value,branch:br.value,html:pg.value,token:tk.value});siteCache=null;done()}
    catch(e){st.textContent=e.message;st.className="bz-err"}b.disabled=false};
  add(card,f,b,st);return card;
}

// Website apply: checks the connection, shows setup when needed, otherwise the Apply button.
// spec: {label, body, confirmOverwrite, input:{label,value,key}}
export async function siteApply(c,domain,spec){
  const box=h("div","apply-box");c.appendChild(box);
  const draw=async()=>{
    box.innerHTML="";
    let s;try{s=await siteStatus(true)}catch(e){box.appendChild(h("p","bz-mut","Apply isn't available right now: "+e.message));return}
    if(!s.connected){box.appendChild(setupCard(bareHost(domain),draw));return}
    if(domain&&bareHost(domain)!==bareHost(s.domain)){
      box.appendChild(h("p","bz-mut","This report is for "+bareHost(domain)+", but your connected website is "+s.domain+". Apply only works on your own site."));return}
    let inp=null;
    if(spec.input){const lab=h("label","bz-mut",spec.input.label);inp=h("input");inp.value=spec.input.value;lab.appendChild(inp);box.appendChild(lab)}
    const body=()=>{const raw=typeof spec.body==="function"?spec.body():spec.body;return JSON.parse(JSON.stringify(raw).replace(/__INPUT__/g,inp?inp.value.replace(/["\\]/g,""):""))};
    const run=async()=>{
      const b=body();
      try{return await api("POST","/api/site/apply",b)}
      catch(e){
        if(String(e.message).trim()==="EXISTS"){
          if(!window.confirm("A "+b.file+" file already exists on your site. Replace it with this one? (You can revert it on GitHub.)"))throw new Error("Cancelled. Your existing file was kept.");
          return await api("POST","/api/site/apply",Object.assign(b,{overwrite:true}))}
        throw e}
    };
    box.appendChild(applyBtn(spec.label||"Apply to my website",run));
    const meta=h("p","bz-mut apply-meta","Connected: "+s.repo+" ("+s.branch+") · ");
    const x=h("a",null,"Disconnect");x.href="#";x.onclick=async ev=>{ev.preventDefault();await api("POST","/api/site/disconnect",{});siteCache=null;draw()};
    meta.appendChild(x);box.appendChild(meta);
  };
  await draw();
}

// Send text to the Publish tab (fills the form, switches tab).
export function sendToPublish(draft){
  window.__bzDraft=draft;
  document.dispatchEvent(new CustomEvent("bz-goto",{detail:{kind:"publish"}}));
}
export const publishBtn=(label,getDraft)=>{const b=h("button","bz-btn",label||"Use in Publish");b.onclick=()=>sendToPublish(getDraft());return b};
