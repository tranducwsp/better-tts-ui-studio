import io
import zipfile
import xml.etree.ElementTree as ET
import PyPDF2
import docx
from fastapi import APIRouter, HTTPException, UploadFile, File, Form, Depends
from core.models import User
from api.auth import get_current_active_user

router = APIRouter()

@router.post("/extract-text")
async def extract_text(
    file: UploadFile = File(...),
    current_user: User = Depends(get_current_active_user)
):
    """Đọc file tài liệu và trả về văn bản bên trong."""
    try:
        content = await file.read()
        filename = file.filename.lower()
        extracted_text = ""

        if filename.endswith('.txt'):
            extracted_text = content.decode('utf-8')
        elif filename.endswith('.pdf'):
            import re
            reader = PyPDF2.PdfReader(io.BytesIO(content))
            for page in reader.pages:
                text_page = page.extract_text()
                if text_page:
                    extracted_text += text_page + "\n"
            
            # Tạm thời tắt tính năng chống rớt dòng ép buộc vì nó làm hỏng định dạng Thơ (gộp các dòng thơ không có dấu chấm thành 1)
            # extracted_text = re.sub(r'(?<=[^\n.!?:"”\-])\n(?!\n)', ' ', extracted_text)
            # extracted_text = re.sub(r' +', ' ', extracted_text)
        elif filename.endswith('.docx'):
            doc = docx.Document(io.BytesIO(content))
            for para in doc.paragraphs:
                extracted_text += para.text + "\n"
        elif filename.endswith('.odt'):
            with zipfile.ZipFile(io.BytesIO(content)) as z:
                content_xml = z.read("content.xml")
                root = ET.fromstring(content_xml)
                
                # Xử lý các thẻ đặc biệt của ODT (khoảng trắng, tab, xuống dòng) 
                # vì ElementTree.itertext() mặc định bỏ qua các thẻ rỗng (VD: <text:s/>)
                for elem in root.iter():
                    if elem.tag.endswith('}s'):
                        # Thẻ <text:s text:c="N"/> đại diện cho N khoảng trắng
                        # Lấy thuộc tính c (count), nếu không có thì mặc định là 1 khoảng trắng
                        c = 1
                        for key, val in elem.attrib.items():
                            if key.endswith('}c'):
                                c = int(val)
                        elem.text = ' ' * c
                    elif elem.tag.endswith('}tab'):
                        elem.text = '\t'
                    elif elem.tag.endswith('}line-break'):
                        elem.text = '\n'
                        
                paragraphs = []
                for elem in root.iter():
                    tag_local = elem.tag.split('}')[-1]
                    if tag_local in ('p', 'h'):
                        text_val = "".join(elem.itertext())
                        if text_val:
                            paragraphs.append(text_val)
                extracted_text = "\n".join(paragraphs) + "\n"
        else:
            raise HTTPException(status_code=400, detail="Định dạng file không hỗ trợ. Chỉ hỗ trợ .txt, .pdf, .docx, .odt")

        return {"text": extracted_text.strip()}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))
