const main=document.querySelector('#main');
const dialog=document.querySelector('#dialog');
const titles={overview:'Overview',bill:'Understand my bill',care:'Find affordable care',prescriptions:'Prescription savings',account:'My account'};
const templates=new Map();
let activePage='',viewVersion=0,toastTimer,profileName='Eleanor Vance';
const escaped=value=>String(value).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const icon=name=>`<span class="material-symbols-outlined" aria-hidden="true">${name}</span>`;
function notify(message){const toast=document.querySelector('#toast');toast.textContent=message;toast.style.display='block';clearTimeout(toastTimer);toastTimer=setTimeout(()=>toast.style.display='none',4500)}
function openDialog(title,content){document.querySelector('#dialog-title').textContent=title;document.querySelector('#dialog-content').innerHTML=content;dialog.showModal()}
document.querySelector('#close-dialog').onclick=()=>dialog.close();
dialog.addEventListener('click',event=>{if(event.target===dialog){const box=dialog.getBoundingClientRect();if(event.clientX<box.left||event.clientX>box.right||event.clientY<box.top||event.clientY>box.bottom)dialog.close()}});
function route(name){if(activePage===name){main.focus({preventScroll:true});return}location.hash=name}
async function render(){
 const name=location.hash.slice(1)||'overview';if(name==='main'){main.focus();return}const page=titles[name]?name:'overview';const version=++viewVersion;activePage=page;
 document.querySelectorAll('[data-page]').forEach(link=>{if(link.dataset.page===page)link.setAttribute('aria-current','page');else link.removeAttribute('aria-current')});
 document.title=`${titles[page]} · ClearCare`;
 try{if(page==='account'){main.innerHTML='';main.className='relative w-full pt-16 bg-surface page-account';window.dispatchEvent(new CustomEvent('clearcare:page',{detail:{page,version}}));window.scrollTo({top:0});return}if(!templates.has(page)){const response=await fetch(`/pages/${page}.html`);if(!response.ok)throw new Error('Unable to load screen');templates.set(page,await response.text())}if(version!==viewVersion)return;main.innerHTML=templates.get(page);main.className=`relative w-full pt-16 bg-surface page-${page}`;wirePage(page);window.dispatchEvent(new CustomEvent('clearcare:page',{detail:{page,version}}));window.scrollTo({top:0});}
 catch{main.innerHTML='<div class="p-10"><h1 class="text-2xl font-bold">This screen couldn’t load.</h1><p>Please refresh the page to try again.</p></div>'}
}
window.addEventListener('hashchange',render);
function setPageButtons(){
 main.querySelectorAll('button').forEach(button=>{
  button.type='button';
  if(button.id||button.classList.contains('category-chip')||button.classList.contains('dosage-option'))return;
  const label=button.textContent.replaceAll('arrow_forward','').trim().toLowerCase();
  if(label.includes('scan claims')||label.includes('active audits')||label.includes('review bill'))button.onclick=()=>route('bill');
  else if(label.includes('dispute letters'))button.onclick=showLetter;
  else if(label.includes('compare local')||label.includes('mri knee')||label.includes('metabolic panel')||label.includes('primary care'))button.onclick=()=>route('care');
  else if(label.includes('manage rx'))button.onclick=()=>route('prescriptions');
  else if(label.includes('book estimate')||label.includes('view details'))button.onclick=()=>providerDialog(button.closest('.bg-surface-container-lowest')?.querySelector('h2')?.textContent||'Bay Area Open Imaging');
  else if(label.includes('expand map'))button.onclick=()=>openDialog('Sample provider map','<img src="/assets/map.svg" alt="Illustrative map of three sample San Francisco care locations" class="w-full rounded-xl"><p>Illustrative locations only. No live map or booking service is connected.</p>');
  else button.onclick=()=>openDialog('Preview this feature',`<p>${escaped(button.textContent.trim())} is shown as part of the frontend design. No request has been sent.</p>`);
 });
 main.querySelectorAll('[data-path]').forEach(link=>{link.href='#'+normalizePath(link.dataset.path)});
 main.querySelectorAll('.material-symbols-outlined').forEach(element=>element.setAttribute('aria-hidden','true'));
}
function normalizePath(path){return path.includes('understand')?'bill':path.includes('affordable')?'care':path.includes('prescription')?'prescriptions':'overview'}
function wirePage(page){setPageButtons();if(page==='bill')wireBill();if(page==='care')wireCare();if(page==='prescriptions')wirePrescriptions();if(page==='overview'){
 const search=main.querySelector('input');if(search){search.setAttribute('aria-label','Search procedures');search.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();route('care')}})}
 const benefits=[...main.querySelectorAll('span')].find(el=>el.textContent.trim()==='Full Benefits →');if(benefits){const button=document.createElement('button');button.className=benefits.className;button.textContent='Full benefits';button.onclick=showBenefits;benefits.replaceWith(button)}
}}
function showBenefits(){openDialog('Sample plan benefits','<div class="sample-note">Illustrative plan — coverage is not verified.</div><table class="price-table"><tbody><tr><th>Annual deductible</th><td>$3,000</td></tr><tr><th>Sample progress</th><td>$1,850</td></tr><tr><th>Out-of-pocket maximum</th><td>$5,200</td></tr></tbody></table><p>Confirm benefits, network participation, and your current deductible directly with your insurer.</p>')}
function providerDialog(name){const records={'Bay Area Open Imaging':{insurance:420,cash:385,distance:'0.8 miles',type:'Independent imaging center'},'Metro Diagnostic Center':{insurance:610,cash:540,distance:'2.4 miles',type:'Freestanding radiology'},'St. Jude Hospital Outpatient':{insurance:1420,cash:1290,distance:'3.1 miles',type:'Hospital outpatient department'}};const data=records[name]||records['Bay Area Open Imaging'];openDialog(name,`<div class="sample-note">Fictional provider listing · No appointment will be booked.</div><p>MRI knee without contrast · CPT 73721</p><p>${escaped(data.type)} · ${escaped(data.distance)}</p><table class="price-table"><tbody><tr><th>Sample plan price</th><td>$${data.insurance.toLocaleString()}</td></tr><tr><th>Sample cash price</th><td>$${data.cash.toLocaleString()}</td></tr></tbody></table><p>Before booking, confirm the total price, whether the radiologist’s interpretation is included, and participation in your exact plan.</p><button class="primary" id="save-estimate">Save sample estimate</button>`);document.querySelector('#save-estimate').onclick=()=>{dialog.close();notify(`Sample estimate for ${name} saved for this preview session.`)}}
function wireBill(){
 // This preview contains no upload input. All entry buttons use the real intake.
 const openIntake=()=>{const form=main.querySelector('#ai-bill-upload');if(!form){notify('The connected bill intake is not ready. Refresh the page or restart the frontend server.');return;}form.scrollIntoView({behavior:'smooth',block:'start'});form.querySelector('input[type=file]').click();};
 main.querySelector('#upload-cta-btn').onclick=openIntake;
 main.querySelector('#sample-bill-btn').onclick=()=>{const review=main.querySelector('[data-source-preview] h2');review?.scrollIntoView({behavior:'smooth',block:'center'});notify('Illustrative design example. Use the real bill intake above to analyze your document.');};
 main.querySelector('#advocate-btn').onclick=()=>openDialog('Prepare for a billing review','<div class="sample-note">No advocate service is connected.</div><p>For the sample statement, ask the billing office to explain the repeated lab collection and the components of the metabolic panel charge.</p><p>Keep your itemized statement and insurer’s EOB ready. A review flag does not establish an error.</p><button class="primary" id="inquiry-from-advocate">Prepare sample inquiry</button>');
 main.querySelector('#dispute-btn').onclick=showLetter;
}
const letter='Subject: Request for itemized statement and billing clarification\n\nHello Billing Team,\n\nI am reviewing the sample statement for account AR-98102. Please explain whether the repeated lab collection charge represents a separate service, and clarify what is included in the metabolic panel charge.\n\nPlease provide an itemized statement and reconcile it with the insurer’s explanation of benefits.\n\nThank you,\nEleanor Vance\n\nDEMONSTRATION ONLY — replace all sample details before use.';
function showLetter(){openDialog('Sample billing inquiry',`<div class="sample-note">Editable frontend example. Nothing is submitted to a provider.</div><label for="letter-text">Inquiry draft</label><textarea id="letter-text">${escaped(letter)}</textarea><div class="flex gap-3 flex-wrap mt-4"><button class="primary" id="print-letter">Print / Save PDF</button><button class="secondary-action" id="download-letter">Download text</button></div>`);document.querySelector('#download-letter').onclick=()=>{const value=document.querySelector('#letter-text').value;const url=URL.createObjectURL(new Blob([value],{type:'text/plain'}));const link=document.createElement('a');link.href=url;link.download='clearcare-sample-inquiry.txt';link.click();setTimeout(()=>URL.revokeObjectURL(url),1000)};document.querySelector('#print-letter').onclick=()=>{document.querySelector('.print-letter')?.remove();const print=document.createElement('article');print.className='print-letter';print.textContent=document.querySelector('#letter-text').value;document.body.append(print);window.print()}}
function wireCare(){
 const input=main.querySelector('#procedure-search');input.setAttribute('aria-label','Procedure or provider search');const locationInput=main.querySelectorAll('input')[1];locationInput.setAttribute('aria-label','ZIP code and search radius');
 const cards=[...main.querySelectorAll('[data-insurance]')].map(price=>price.closest('.bg-surface-container-lowest'));
 const container=cards[0].parentElement;container.id='provider-list';const count=container.previousElementSibling.querySelector('.font-headline');count.setAttribute('aria-live','polite');
 const empty=document.createElement('div');empty.className='empty-results';empty.hidden=true;empty.textContent='No sample listings match. Try MRI or select Imaging & MRI.';container.append(empty);
 let category='Imaging & MRI';
 const filter=()=>{const value=input.value.toLowerCase().trim(),zip=locationInput.value.trim();let matches=0;cards.forEach(card=>{const query=card.textContent.toLowerCase()+' mri knee without contrast imaging cpt 73721';const visible=category==='Imaging & MRI'&&(!value||value.split(/\s+/).every(term=>query.includes(term.replace(/[,.]/g,''))))&&(!zip||zip.includes('94107')||zip.toLowerCase().includes('san francisco'));card.hidden=!visible;if(visible)matches++});empty.hidden=matches>0;count.textContent=`${matches} sample results for “${input.value||category}”`};
 input.addEventListener('input',filter);locationInput.addEventListener('input',filter);
 main.querySelectorAll('.category-chip').forEach(chip=>{const label=chip.querySelector('span:last-child')?.textContent||chip.textContent;chip.dataset.category=label.trim();chip.setAttribute('aria-pressed',chip.dataset.category===category?'true':'false');chip.onclick=()=>{category=chip.dataset.category;main.querySelectorAll('.category-chip').forEach(c=>c.setAttribute('aria-pressed',c===chip?'true':'false'));if(category==='Imaging & MRI')input.value='MRI Knee';else input.value='';filter()}});
 function mode(kind){main.querySelectorAll('[data-insurance]').forEach(price=>price.textContent=price.dataset[kind]);main.querySelectorAll('[data-sub-insurance]').forEach(text=>text.textContent=text.dataset[kind==='cash'?'subCash':'subInsurance']);for(const name of ['insurance','cash']){const button=main.querySelector('#btn-'+name);button.classList.toggle('mode-active',name===kind);button.setAttribute('aria-pressed',String(name===kind))}}
 main.querySelector('#btn-insurance').onclick=()=>mode('insurance');main.querySelector('#btn-cash').onclick=()=>mode('cash');mode('insurance');
}
function wirePrescriptions(){
 const search=main.querySelector('#med-search-input');search.setAttribute('aria-label','Search active prescriptions');const list=main.querySelector('#rx-list');const empty=document.createElement('p');empty.className='empty-results';empty.hidden=true;empty.textContent='No sample prescriptions match that name.';list.append(empty);
 search.oninput=()=>{updateComparison();let count=0;list.querySelectorAll('.rx-card').forEach(card=>{card.hidden=!card.dataset.name.includes(search.value.toLowerCase().trim());if(!card.hidden)count++});empty.hidden=count>0};
 const dosage=main.querySelector('#dosage-btn'),menu=main.querySelector('#dosage-menu');dosage.setAttribute('aria-expanded','false');dosage.setAttribute('aria-controls','dosage-menu');dosage.onclick=()=>{const hidden=menu.classList.toggle('hidden');dosage.setAttribute('aria-expanded',String(!hidden))};
 function updateComparison(){
  const table=main.querySelector('#pharmacy-table');
  const quantity=main.querySelector('#dosage-label').textContent;
  const query=search.value.toLowerCase().trim();
  const supported=quantity==='20mg • 30 tablets'&&(!query||'atorvastatin calcium lipitor'.includes(query));
  table.hidden=!supported;main.querySelector('#dosage-empty')?.remove();
  if(!supported){const message=document.createElement('p');message.id='dosage-empty';message.className='empty-results';message.textContent='The sample pharmacy comparison is available for Atorvastatin 20mg · 30 tablets. Clear the search and restore that dosage to view it.';table.after(message)}
 }
 menu.querySelectorAll('.dosage-option').forEach(option=>option.onclick=()=>{main.querySelector('#dosage-label').textContent=option.dataset.dosage;menu.classList.add('hidden');dosage.setAttribute('aria-expanded','false');updateComparison()});
 main.querySelector('#activate-coupon-btn').onclick=()=>openDialog('Preview a prescription coupon','<div class="sample-note">Demonstration only — not redeemable at a pharmacy.</div><h3 class="text-lg font-bold">Atorvastatin Calcium</h3><p>20mg · 30 tablets</p><table class="price-table"><tbody><tr><th>Sample pharmacy</th><td>CVS Pharmacy</td></tr><tr><th>Illustrative cash price</th><td>$9.40</td></tr></tbody></table><p>A real coupon requires a pricing-provider integration. Confirm the exact product and price with your pharmacist.</p>');
 main.querySelector('#check-eligibility-btn').onclick=()=>openDialog('Copay assistance eligibility','<div class="sample-note">Eligibility is not checked in this frontend preview.</div><p>Program rules may depend on insurance type, location, and other criteria. Check the manufacturer’s program requirements before relying on an offer.</p>');
}
document.addEventListener('click',event=>{const action=event.target.closest('[data-action]')?.dataset.action;if(action==='settings')showProfile();if(action==='notifications')openDialog('Your care updates','<p>No live notifications are available. Check your account for saved reviews and insurance analysis.</p>');if(action==='search')showSearch();if(event.target.id==='inquiry-from-advocate'){dialog.close();showLetter()}});
function showProfile(){openDialog('Demo profile',`<div class="sample-note">Preview settings only. No account is created or saved to a server.</div><form id="profile-form"><label>Display name<input name="name" value="${escaped(profileName)}" required maxlength="80"></label><label>Plan<input value="BlueCross Shield Direct" readonly></label><button class="primary">Apply to this preview</button></form>`);document.querySelector('#profile-form').onsubmit=event=>{event.preventDefault();profileName=new FormData(event.target).get('name').trim()||'Eleanor Vance';document.querySelectorAll('[data-profile-name]').forEach(item=>item.textContent=profileName);dialog.close();notify('Demo profile updated for this session.')}}
function showSearch(){openDialog('Where would you like to go?','<label>Search ClearCare<input id="global-search" placeholder="Bill, care, prescriptions…" autocomplete="off"></label><div class="search-options"><button data-go="bill">Understand my bill</button><button data-go="care">Find affordable care</button><button data-go="prescriptions">Prescription savings</button></div>');const input=document.querySelector('#global-search');const options=[...dialog.querySelectorAll('[data-go]')];input.oninput=()=>options.forEach(option=>option.hidden=!option.textContent.toLowerCase().includes(input.value.toLowerCase()));options.forEach(option=>option.onclick=()=>{dialog.close();route(option.dataset.go)});input.focus()}
render();
