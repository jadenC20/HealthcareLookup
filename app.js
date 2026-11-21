const main=document.querySelector('#main');
const btn=(text,page,cls='primary')=>`<button class="${cls}" data-page="${page}">${text}</button>`;
function showModal(html){document.querySelector('#modal-content').innerHTML=html;document.querySelector('#modal').showModal()}
function toast(text){const t=document.querySelector('#toast');t.textContent=text;t.style.display='block';setTimeout(()=>t.style.display='none',4000)}
function dashboard(){main.innerHTML=`<div class="greeting"><div><h1>A little clarity. A healthier you.</h1><p class="subtitle">Your next step toward more affordable healthcare starts here.</p></div><span class="date">YOUR PERSONAL CARE COMPANION</span></div><section class="hero"><div><span class="eyebrow">LESS CONFUSION. MORE CONFIDENCE.</span><h2>Good care shouldn’t<br>come with a question mark.</h2><p>Make sense of your medical bills and explore care that works for your budget.</p>${btn('Understand my bill','bills')}</div><div class="hero-art" aria-hidden="true"><div class="orbit"></div><div class="orbit two"></div><div class="art-cross">✚</div><div class="floating">▤ &nbsp; Your costs, made clear</div><div class="floating last"><b>✓</b> &nbsp; A more confident next step</div></div></section><div class="section-title"><h2>How can we help today?</h2><span>A clearer path, at every step</span></div><section class="action-grid"><button class="action-card" data-page="bills"><span class="tile-icon">▤</span><h3>Understand my bill</h3><p>Break down confusing charges and find questions worth asking.</p><span class="card-link">Review a bill</span></button><button class="action-card" data-page="care"><span class="tile-icon blue">⌕</span><h3>Find affordable care</h3><p>Explore providers and compare costs for your next appointment.</p><span class="card-link">Explore care options</span></button><button class="action-card" data-page="meds"><span class="tile-icon orange">✚</span><h3>Prescription information</h3><p>Medication pricing integration is coming next.</p><span class="card-link">View availability</span></button></section><section class="lower-grid"><div class="panel"><h2>Your bill review, at a glance</h2><span class="muted">Try a guided example to see what ClearCare can surface.</span><div class="bill-row"><span class="tile-icon">▤</span><div><strong>Sample imaging visit</strong><small>Illustrative bill + insurance statement</small></div><div class="amount"><strong>$640.00</strong><small>Bill balance</small></div></div><div class="bill-bottom"><span class="tag">Backend sample</span><button class="text-btn" id="open-sample">Open sample review</button></div></div><div class="panel tip"><span class="tip-label">A LITTLE HEALTHCARE KNOW-HOW</span><h3>An explanation of benefits isn’t a bill.</h3><p>Compare your insurer’s statement with your provider’s bill before you pay. Differences are a reason to ask questions.</p><button class="text-btn" id="learn-eob">What should I compare?</button></div></section>`}

function bills(){} // Implemented by the API integration module.
function searchPage(){}
function navigate(page){
 document.querySelectorAll('.nav').forEach(b=>b.classList.toggle('active',b.dataset.page===page));
 document.querySelector('#crumb').textContent={dashboard:'Overview',bills:'Understand my bill',care:'Find affordable care',meds:'Prescription information'}[page]||'Bill review';
 if(page==='dashboard')dashboard();else if(page==='bills')bills();else if(page==='care'||page==='meds')searchPage(page);
 window.scrollTo({top:0,behavior:'smooth'});
}
document.addEventListener('click',e=>{
 const nav=e.target.closest('[data-page]');
 if(nav){navigate(nav.dataset.page);return}
 if(e.target.id==='open-sample')navigate('sample');
 if(e.target.id==='learn-eob')showModal('<h2>Compare the same claim.</h2><p>Check service dates, provider, claim number, payments, adjustments, and patient responsibility. Ask the billing office to explain differences.</p>');
});
document.querySelector('.close').onclick=()=>document.querySelector('#modal').close();
document.querySelector('.brand').onclick=e=>{e.preventDefault();navigate('dashboard')};
navigate('dashboard');
