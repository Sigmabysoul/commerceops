"use client";

import { FormEvent, useEffect, useState } from "react";
import { flipkartAPI, JobDetails } from "@/api/flipkart";

export function FlipkartProcessing(){
  const [details,setDetails]=useState<JobDetails|null>(null);const [jobID,setJobID]=useState("");const [error,setError]=useState("");const [uploading,setUploading]=useState(false);const [duplicate,setDuplicate]=useState(false);
  useEffect(()=>{if(!jobID||details?.job.status==="processed"||details?.job.status==="needs_review"||details?.job.status==="failed")return;const timer=setInterval(()=>flipkartAPI.job(jobID).then(setDetails).catch(cause=>setError(message(cause))),1200);return()=>clearInterval(timer)},[jobID,details?.job.status]);
  async function upload(event:FormEvent<HTMLFormElement>){event.preventDefault();const form=event.currentTarget;const file=(new FormData(form).get("file"));if(!(file instanceof File)||!file.size)return;setUploading(true);setError("");setDetails(null);try{const result=await flipkartAPI.upload(file);setJobID(result.job.id);setDuplicate(result.duplicate_source);setDetails(await flipkartAPI.job(result.job.id));form.reset()}catch(cause){setError(message(cause))}finally{setUploading(false)}}
  return <section className="flipkart"><div className="product-heading"><div><p className="eyebrow">Phase 3</p><h2>Flipkart processing</h2><p className="muted">Upload a Flipkart PDF. Processing continues in a bounded background queue.</p></div></div>
    <section className="panel"><form className="upload-row" onSubmit={upload}><label>Flipkart PDF<input name="file" type="file" accept="application/pdf,.pdf" required /></label><button disabled={uploading}>{uploading?"Uploading…":"Upload and process"}</button></form>{error&&<p className="error" role="alert">{error}</p>}{duplicate&&<p className="notice">This exact source file was already uploaded. Showing its existing job.</p>}</section>
    {details&&<section className="panel results"><div className="status-line"><h2>Processing results</h2><span className={`status status-${details.job.status}`}>{details.job.status.replace("_"," ")}</span></div><p className="muted">{details.job.processed_pages}/{details.job.total_pages} pages · parser {details.job.parser_version}</p>{["needs_review","failed","processed"].includes(details.job.status)&&<button onClick={()=>flipkartAPI.retry(details.job.id).then(result=>{setDetails({...details,job:result.job,orders:[],errors:[]});setJobID(result.job.id)}).catch(cause=>setError(message(cause)))}>Reprocess with current SKU training</button>}
      {details.orders.map(order=><article key={order.id}><div><strong>Page {order.source_page}</strong> · {order.awb??"AWB missing"}<small>{order.marketplace_order_id??"Order ID missing"} · {order.status}</small></div>{order.items.map((item,index)=><div key={index}><span>{item.raw_sku??"SKU missing"} → {item.product_id??"Product training required"}</span><small>Quantity: {item.quantity??"unknown"} ({item.quantity_source}) · {item.resolution_status}</small></div>)}</article>)}
      {details.errors.length>0&&<div className="review"><h3>Warnings and errors</h3><ul>{details.errors.map((item,index)=><li key={`${item.code}-${index}`}><span>{item.code}<small>{item.source_page?`Page ${item.source_page} · `:""}{item.message}</small></span></li>)}</ul></div>}
    </section>}
  </section>
}
function message(cause:unknown){return cause instanceof Error?cause.message:"Something went wrong"}
