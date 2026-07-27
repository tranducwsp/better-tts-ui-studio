import logging
import uuid6
from core.database import engine, Base, SessionLocal
from core.models import User
from core.security import get_password_hash

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

def init_db():
    import os
    from dotenv import load_dotenv
    load_dotenv()

    logger.info("Đang tạo các bảng trong CSDL...")
    Base.metadata.create_all(bind=engine)
    
    db = SessionLocal()
    try:
        # Danh sách tài khoản mặc định
        accounts = []
        admin_user = os.getenv("DEFAULT_ADMIN_USERNAME")
        admin_pass = os.getenv("DEFAULT_ADMIN_PASSWORD")
        if admin_user and admin_pass:
            accounts.append({"username": admin_user, "password": admin_pass, "role": "admin"})
            
        default_user = os.getenv("DEFAULT_USER_USERNAME")
        default_pass = os.getenv("DEFAULT_USER_PASSWORD")
        if default_user and default_pass:
            accounts.append({"username": default_user, "password": default_pass, "role": "user"})

        
        for acc in accounts:
            user = db.query(User).filter(User.username == acc["username"]).first()
            if not user:
                logger.info(f"Đang tạo tài khoản {acc['role']}: {acc['username']}")
                hashed_pw = get_password_hash(acc["password"])
                new_user = User(
                    id=str(uuid6.uuid7()),
                    username=acc["username"],
                    password_hash=hashed_pw,
                    role=acc["role"],
                    is_approved=True
                )
                db.add(new_user)
            else:
                logger.info(f"Tài khoản '{acc['username']}' đã tồn tại.")
        
        db.commit()
        logger.info("Khởi tạo Database thành công!")
    except Exception as e:
        logger.error(f"Lỗi khi khởi tạo DB: {e}")
    finally:
        db.close()

if __name__ == "__main__":
    init_db()
